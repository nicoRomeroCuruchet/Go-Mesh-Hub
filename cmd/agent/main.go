package main

import (
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"time"

	"github.com/songgao/water"
	"golang.org/x/crypto/chacha20poly1305"
)

var (
	hubIP      = flag.String("hub-ip", "", "Public IP of the Hub Server")
	hubPort    = flag.Int("hub-port", 5000, "UDP port of the Hub")
	tunIP      = flag.String("tun-ip", "", "My Virtual IP (e.g. 10.0.0.2)")
	secret    = flag.String("secret", "change-this-password", "Shared secret for encryption")
)

func main() {
	flag.Parse()
	if *hubIP == "" || *tunIP == "" {
		log.Fatal("Usage: sudo ./client -hub-ip <IP> -tun-ip <IP>")
	}

	// 1. Crypto
	key := sha256.Sum256([]byte(*secret))
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		log.Fatal(err)
	}

	// 2. TUN
	config := water.Config{DeviceType: water.TUN}
	ifce, err := water.New(config)
	if err != nil {
		log.Fatal(err)
	}
	setupInterface(ifce.Name(), *tunIP)

	// 3. UDP Connection to Hub
	serverAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *hubIP, *hubPort))
	if err != nil {
		log.Fatal(err)
	}
	// DialUDP fixes the remote address, so we don't need to specify it every time
	conn, err := net.DialUDP("udp", nil, serverAddr) 
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Printf("Client %s started. Connected to Hub at %s\n", *tunIP, serverAddr)
    
	// HANDSHAKE Initial 
    go func() {
		// 1. Construct a minimal, valid IPv4 Header (20 bytes)
		// We don't need a payload, just the headers for the Server's Deep Packet Inspection.
		handshakePacket := make([]byte, 20)

		// Byte 0: Version (4) + Header Length (5 words = 20 bytes) = 0x45
		// CRITICAL: The Server ignores packets if version != 4.
		handshakePacket[0] = 0x45

		// Bytes 12-16: Source IP (My Virtual Identity)
		// The server uses this to learn "Who I am"
		sourceIP := net.ParseIP(*tunIP).To4()
		copy(handshakePacket[12:16], sourceIP)

		// Bytes 16-20: Destination IP (Hub Virtual Identity)
		// This is formal; the server will consume it anyway. 
		// We assume the Hub is usually the .1 address.
		destIP := net.ParseIP("10.0.0.1").To4() 
		copy(handshakePacket[16:20], destIP)

		// 2. Encrypt the Packet
		nonce := make([]byte, aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			log.Printf("[ERR] Handshake failed: Could not generate nonce: %v", err)
			return
		}

		// Seal (Encrypt + Authenticate)
		encryptedPayload := aead.Seal(nil, nonce, handshakePacket, nil)

		// 3. Transmit: [Nonce + Encrypted Data]
		finalPacket := append(nonce, encryptedPayload...)
		if _, err := conn.Write(finalPacket); err != nil {
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
			nonce := make([]byte, aead.NonceSize())
			io.ReadFull(rand.Reader, nonce)
			// Encrypting an empty slice []byte{}
			heartbeat := aead.Seal(nil, nonce, []byte{}, nil)
			conn.Write(append(nonce, heartbeat...))
		}
	}()

	// --- INBOUND LOOP (Hub -> TUN) ---
	go func() {
		buf := make([]byte, 2000)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			
			// Decrypt
			nonceSize := aead.NonceSize()
			if n < nonceSize { continue }
			nonce := buf[:nonceSize]
			ciphertext := buf[nonceSize:n]
			plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
			if err != nil {
				continue
			}

			// Write to TUN (only if it's a valid packet)
			if len(plaintext) > 0 {
				ifce.Write(plaintext)
			}
		}
	}()

	// --- OUTBOUND LOOP (TUN -> Hub) ---
	packet := make([]byte, 2000)
	for {
		n, err := ifce.Read(packet)
		if err != nil {
			log.Fatal(err)
		}
		// Encrypt and send everything to Hub
		nonce := make([]byte, aead.NonceSize())
		io.ReadFull(rand.Reader, nonce)
		encrypted := aead.Seal(nil, nonce, packet[:n], nil)
		conn.Write(append(nonce, encrypted...))
	}
}

func setupInterface(ifaceName, ip string) {
	cidr := ip + "/24"
	exec.Command("ip", "addr", "add", cidr, "dev", ifaceName).Run()
	exec.Command("ip", "link", "set", "dev", ifaceName, "mtu", "1300").Run()
	exec.Command("ip", "link", "set", "dev", ifaceName, "up").Run()
}