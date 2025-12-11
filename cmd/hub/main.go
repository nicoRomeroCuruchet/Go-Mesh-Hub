package main

import (
	"fmt"
	"log"
	"net"

	"go-mesh-hub/internal/config"
	"go-mesh-hub/internal/dashboard"
	"go-mesh-hub/internal/router"
	"go-mesh-hub/internal/security"
	"go-mesh-hub/internal/tun"
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

	// --- EXIT NODE CONFIGURATION ---
    if cfg.ExitNodeIP == cfg.TunIP {
        // We call our new function
        cleanupNAT, err := tun.EnableExitNode(ifce.Name())
        if err != nil {
            log.Fatalf("[CRIT] Failed to enable Exit Node: %v", err)
        }
		log.Printf("[INFO] Exit node on %s", cfg.ExitNodeIP)
        // IMPORTANT: Ensure rules are deleted when we kill the app
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

			plaintext, err := sec.DecryptUnpack(buf[:n])
			if err != nil {
				continue // Auth fail
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
				routeTable.RecordRx(srcIP, len(plaintext)) // Update Dashboard Stats

				// B. Routing Decision
                isPeer := routeTable.Lookup(dstIP) != nil

                if isPeer {
					//It's internal VPN traffic
                    forwardPacket(plaintext, dstIP, conn, sec, routeTable)
                
                } else if dstIP == cfg.TunIP {
                    // It's for me: Eg. ping to Hub
                    ifce.Write(plaintext)
                
                } else if cfg.ExitNodeIP == cfg.TunIP {
			        //It's Internet traffic! (e.g., Destination 8.8.8.8)
				    //Since I'm the Exit Node and I've already enabled NAT, I inject the packet
				    //into my TUN interface. The Linux kernel will see that it's for 8.8.8.8
				    //and will route it through eth0 using Masquerade.
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
			log.Fatalf("[CRIT] TUN Read Error: %v", err)
		}
		
		if n < 20 { continue }

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

	// re-encryping
	encryptedData, err := sec.PackAndEncrypt(data)
	if err != nil {
		return
	}

	conn.WriteToUDP(encryptedData, targetAddr)
	
	// Update Dashboard Stats (Tx)
	table.RecordTx(dstIP, len(data)) 
}
