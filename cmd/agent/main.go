package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go-mesh-hub/internal/protocol"
	"go-mesh-hub/internal/security"
	"go-mesh-hub/internal/tun"
)

var (
	hubIP       = flag.String("hub-ip", "", "Public IP of the Hub Server")
	hubPort     = flag.Int("hub-port", 5000, "UDP port of the Hub")
	hubTunIP    = flag.String("hub-tun-ip", "10.0.0.1", "Virtual IP of the Hub (its -tun-ip, e.g. 10.192.168.1)")
	tunIP       = flag.String("tun-ip", "", "My Virtual IP (e.g. 10.0.0.2)")
	isExitNode  = flag.Bool("exit-node", false, "Act as an Exit Node (Route internet traffic)")
	useExitNode = flag.Bool("global-exit", false, "Route all internet traffic through the VPN Hub")
	secret      = flag.String("secret", "change-this-password", "Shared secret for encryption")
)

var (
	directPeersMu sync.RWMutex
	directPeers   = make(map[string]*net.UDPAddr) // virtualIP → real addr
)

func main() {
	flag.Parse()
	if *hubIP == "" || *tunIP == "" {
		log.Fatal("Usage: sudo ./client -hub-ip <IP> -tun-ip <IP>")
	}

	// 1. Crypto
	sec, err := security.New(*secret)
	if err != nil {
		log.Fatal(err)
	}

	// 2. TUN
	ifce, err := tun.Setup(*tunIP)
	if err != nil {
		log.Fatalf("[CRIT] TUN init failed: %v", err)
	}

	var cleanupNAT func()
	// --- EXIT NODE CONFIGURATION ---
	if *isExitNode {
		cleanupNAT, err = tun.EnableExitNode(ifce.Name())
		if err != nil {
			log.Fatalf("[CRIT] Failed to enable Exit Node: %v", err)
		}
		// IMPORTANT: Ensure rules are deleted when we kill the app
		defer cleanupNAT()
	}

	// if useExitNode we have to redirect all the trafic
	if *useExitNode {
		// Resolvemos la IP del Hub (si nos pasaron un dominio, necesitamos la IP numérica para 'ip route')
		hubUDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *hubIP, *hubPort))
		if err != nil {
			log.Fatalf("[CRIT] Failed to resolve Hub IP: %v", err)
		}
		hubRealIP := hubUDPAddr.IP.String()
		// Magic happens here:
		cleanupRoutes, err := tun.RedirectGateway(ifce.Name(), hubRealIP)
		if err != nil {
			log.Fatalf("[CRIT] Failed to redirect gateway: %v", err)
		}
		defer cleanupRoutes() // Restore internet when we exit
		log.Println("[INFO] Global Exit Node active. You are now surfing via the Hub.")
	}

	// 3. UDP Socket (ListenUDP — can receive from any source for P2P)
	hubAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *hubIP, *hubPort))
	if err != nil {
		log.Fatal(err)
	}
	localUDPAddr, _ := net.ResolveUDPAddr("udp", ":0") // OS-assigned port
	conn, err := net.ListenUDP("udp", localUDPAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Printf("Client %s started. Connected to Hub at %s\n", *tunIP, hubAddr)

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

	// HANDSHAKE Initial
	go func() {
		// 1. Construct a minimal, valid IPv4 Header (20 bytes)
		handshakePacket := make([]byte, 20)

		// Byte 0: Version (4) + Header Length (5 words = 20 bytes) = 0x45
		handshakePacket[0] = 0x45

		// Bytes 12-16: Source IP (My Virtual Identity)
		sourceIP := net.ParseIP(*tunIP).To4()
		copy(handshakePacket[12:16], sourceIP)

		// Bytes 16-20: Destination IP (Hub Virtual Identity)
		destIP := net.ParseIP(*hubTunIP).To4()
		copy(handshakePacket[16:20], destIP)

		// 2. Encrypt and send with MsgData type prefix
		encrypted, err := sec.PackAndEncrypt(handshakePacket)
		if err != nil {
			log.Printf("[ERR] Handshake failed: %v", err)
			return
		}
		msg := append([]byte{protocol.MsgData}, encrypted...)
		if _, err := conn.WriteToUDP(msg, hubAddr); err != nil {
			log.Printf("[ERR] Failed to send Handshake packet: %v", err)
		} else {
			log.Printf("[NET] Handshake sent. Registered Virtual IP %s with Hub.", *tunIP)
		}
	}()

	// --- HEARTBEAT / KEEP-ALIVE ---
	// Sends an empty encrypted packet every 20s to keep NAT open
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		for range ticker.C {
			encrypted, err := sec.PackAndEncrypt([]byte{})
			if err != nil {
				continue
			}
			conn.WriteToUDP(append([]byte{protocol.MsgData}, encrypted...), hubAddr)
		}
	}()

	// --- INBOUND LOOP (Hub/Peers -> TUN) ---
	go func() {
		buf := make([]byte, 2000)
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
				continue
			}

			switch msgType {
			case protocol.MsgData:
				if len(plaintext) > 0 {
					ifce.Write(plaintext)
				}

			case protocol.MsgPeerInfo:
				virtualIP, peerAddr, err := protocol.DecodePeerInfo(plaintext)
				if err != nil {
					continue
				}
				directPeersMu.Lock()
				directPeers[virtualIP.String()] = peerAddr
				directPeersMu.Unlock()
				// Send HOLE_PUNCH to peerAddr so their NAT opens too
				hp, err := sec.PackAndEncrypt(protocol.EncodeHolePunch(net.ParseIP(*tunIP).To4()))
				if err == nil {
					conn.WriteToUDP(append([]byte{protocol.MsgHolePunch}, hp...), peerAddr)
				}
				log.Printf("[P2P] Punching hole to %s at %s", virtualIP, peerAddr)

			case protocol.MsgHolePunch:
				virtualIP, err := protocol.DecodeHolePunch(plaintext)
				if err != nil {
					continue
				}
				directPeersMu.Lock()
				directPeers[virtualIP.String()] = remoteAddr
				directPeersMu.Unlock()
				log.Printf("[P2P] Direct route established with %s", virtualIP)
			}
		}
	}()

	// --- OUTBOUND LOOP (TUN -> Hub/Peers) ---
	packet := make([]byte, 2000)
	for {
		n, err := ifce.Read(packet)
		if err != nil {
			log.Fatal(err)
		}
		if n < 20 {
			continue
		}

		dstIP := net.IP(packet[16:20]).String()

		directPeersMu.RLock()
		peerAddr, isDirect := directPeers[dstIP]
		directPeersMu.RUnlock()

		encrypted, err := sec.PackAndEncrypt(packet[:n])
		if err != nil {
			continue
		}
		msg := append([]byte{protocol.MsgData}, encrypted...)

		if isDirect {
			conn.WriteToUDP(msg, peerAddr)
		} else {
			conn.WriteToUDP(msg, hubAddr)
		}
	}
}

func setupInterface(ifaceName, ip string) {
	cidr := ip + "/24"
	exec.Command("ip", "addr", "add", cidr, "dev", ifaceName).Run()
	exec.Command("ip", "link", "set", "dev", ifaceName, "mtu", "1300").Run()
	exec.Command("ip", "link", "set", "dev", ifaceName, "up").Run()
}
