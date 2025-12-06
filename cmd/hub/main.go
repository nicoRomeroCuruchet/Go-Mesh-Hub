package main

import (
	"fmt"
	"log"
	"net"

	"go-mesh-hub/internal/config"
	"go-mesh-hub/internal/dashboard" // NEW IMPORT
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

				// B. Route
				if dstIP == cfg.TunIP {
					ifce.Write(plaintext)
				} else {
					forwardPacket(plaintext, dstIP, conn, sec, routeTable)
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

// forwardPacket encrypts and sends data
func forwardPacket(data []byte, dstIP string, conn *net.UDPConn, sec *security.Manager, table *router.Table) {
	target := table.Lookup(dstIP)
	if target == nil {
		return
	}

	encryptedData, err := sec.PackAndEncrypt(data)
	if err != nil {
		return
	}

	conn.WriteToUDP(encryptedData, target)
	
	// Update Dashboard Stats (Tx)
	table.RecordTx(dstIP, len(data)) 
}