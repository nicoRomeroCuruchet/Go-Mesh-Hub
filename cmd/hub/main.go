package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go-mesh-hub/internal/config"
	"go-mesh-hub/internal/dashboard"
	"go-mesh-hub/internal/protocol"
	"go-mesh-hub/internal/router"
	"go-mesh-hub/internal/security"
	"go-mesh-hub/internal/tun"
)

var (
	peerInfoMu   sync.Mutex
	lastPeerInfo = make(map[string]time.Time) // key: "srcVIP,dstVIP"
)

func main() {
	// 1. Load Configuration
	cfg := config.Load()

	// 2. Initialize Security
	sec, err := security.New(cfg.Secret)
	if err != nil {
		log.Fatalf("[CRIT] Crypto init failed: %v", err)
	}

	// 3. Initialize TUN
	ifce, err := tun.Setup(cfg.TunIP)
	if err != nil {
		log.Fatalf("[CRIT] TUN setup failed: %v", err)
	}

	// 4. Initialize Routing Table
	routeTable := router.NewTable()
	if cfg.ExitNodeIP != "" {
		routeTable.SetExitNode(cfg.ExitNodeIP)
	}

	var cleanupNAT func()
	// --- EXIT NODE CONFIGURATION ---
	if cfg.ExitNodeIP == cfg.TunIP {
		cleanupNAT, err = tun.EnableExitNode(ifce.Name())
		if err != nil {
			log.Fatalf("[CRIT] Failed to enable Exit Node: %v", err)
		}
		log.Printf("[NAT] Exit Node Enabled on %s. Traffic will be masqueraded via host interface.", cfg.ExitNodeIP)
		defer cleanupNAT()
	}

	// 5. Start UDP Listener
	localAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", cfg.LocalPort))
	if err != nil {
		log.Fatalf("[CRIT] UDP resolve failed: %v", err)
	}
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		log.Fatalf("[CRIT] UDP listen failed: %v", err)
	}
	defer conn.Close()
	log.Printf("[INFO] VPN Server listening on %s", localAddr)

	// For clean the iptable to restore internet
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan

		fmt.Println()
		log.Printf("[OS] Received signal: %v. Cleaning up...", sig)
		// 1. Restore IPTables / NAT
		if cleanupNAT != nil {
			cleanupNAT()
		}
		// 2. close conections
		conn.Close()
		ifce.Close()
		log.Println("[OS] Cleanup complete. Exiting.")
		os.Exit(0)
	}()

	// 6. START DASHBOARD (Non-blocking)
	go dashboard.Start(cfg.WebPort, routeTable)

	// --- LOOP 1: INBOUND (Internet -> Decrypt -> TUN) ---
	go func() {
		buf := make([]byte, 2048)
		for {
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			if n < 1 {
				continue
			}

			msgType := buf[0]
			plaintext, err := sec.DecryptUnpack(buf[1:n])
			if err != nil {
				continue // Auth fail
			}

			// Handle HOLE_PUNCH: learn the route (NAT binding already created by the packet)
			if msgType == protocol.MsgHolePunch {
				vip, err := protocol.DecodeHolePunch(plaintext)
				if err == nil {
					routeTable.Learn(vip.String(), remoteAddr)
				}
				continue
			}

			// Drop non-DATA messages
			if msgType != protocol.MsgData {
				continue
			}

			if len(plaintext) == 0 {
				continue // Heartbeat
			}

			// IPv4 Inspection
			if len(plaintext) >= 20 {
				srcIP := net.IP(plaintext[12:16]).String()
				dstIP := net.IP(plaintext[16:20]).String()

				if srcIP == "0.0.0.0" {
					continue
				}

				// A. Learn Route & Record Stats
				routeTable.Learn(srcIP, remoteAddr)
				routeTable.RecordRx(srcIP, len(plaintext))

				// B. Routing Decision
				isPeer := routeTable.Lookup(dstIP) != nil

				if isPeer {
					// It's internal VPN traffic
					forwardPacket(plaintext, dstIP, conn, sec, routeTable)
					maybeSendPeerInfo(srcIP, dstIP, conn, sec, routeTable)

				} else if dstIP == cfg.TunIP {
					// It's for me: Eg. ping to Hub
					ifce.Write(plaintext)

				} else if cfg.ExitNodeIP == cfg.TunIP {
					// It's Internet traffic! (e.g., Destination 8.8.8.8)
					ifce.Write(plaintext)

				} else {
					log.Printf("Drop: Unknown destination %s", dstIP)
				}
			}
		}
	}()

	// --- LOOP 2: OUTBOUND (TUN -> Encrypt -> Internet) ---
	packet := make([]byte, 2048)
	for {
		n, err := ifce.Read(packet)
		if err != nil {
			return
		}

		if n < 20 {
			continue
		}

		dstIP := net.IP(packet[16:20]).String()
		forwardPacket(packet[:n], dstIP, conn, sec, routeTable)
	}
}

// forwardPacket handles encryption and transmission based on routing rules
func forwardPacket(data []byte, dstIP string, conn *net.UDPConn, sec *security.Manager, table *router.Table) {
	targetAddr, found := table.GetRoute(dstIP)
	if !found {
		// Drop: No route to host (neither Peer nor Exit Node)
		return
	}

	encrypted, err := sec.PackAndEncrypt(data)
	if err != nil {
		return
	}

	packet := append([]byte{protocol.MsgData}, encrypted...)
	conn.WriteToUDP(packet, targetAddr)

	// Update Dashboard Stats (Tx)
	table.RecordTx(dstIP, len(data))
}

// maybeSendPeerInfo sends PEER_INFO to both agents if not sent within the last 30s
func maybeSendPeerInfo(srcIP, dstIP string, conn *net.UDPConn, sec *security.Manager, table *router.Table) {
	key := srcIP + "," + dstIP

	peerInfoMu.Lock()
	if t, ok := lastPeerInfo[key]; ok && time.Since(t) < 30*time.Second {
		peerInfoMu.Unlock()
		return
	}
	lastPeerInfo[key] = time.Now()
	peerInfoMu.Unlock()

	srcAddr := table.Lookup(srcIP)
	dstAddr := table.Lookup(dstIP)
	if srcAddr == nil || dstAddr == nil {
		return
	}

	// Send B's (dstIP) info to A (srcIP)
	payload := protocol.EncodePeerInfo(net.ParseIP(dstIP).To4(), dstAddr)
	if encrypted, err := sec.PackAndEncrypt(payload); err == nil {
		msg := append([]byte{protocol.MsgPeerInfo}, encrypted...)
		conn.WriteToUDP(msg, srcAddr)
	}

	// Send A's (srcIP) info to B (dstIP)
	payload = protocol.EncodePeerInfo(net.ParseIP(srcIP).To4(), srcAddr)
	if encrypted, err := sec.PackAndEncrypt(payload); err == nil {
		msg := append([]byte{protocol.MsgPeerInfo}, encrypted...)
		conn.WriteToUDP(msg, dstAddr)
	}

	log.Printf("[P2P] Sent PEER_INFO for %s <-> %s", srcIP, dstIP)
}
