package tun

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

// RedirectGateway forces all internet traffic through the TUN interface.
// hubRealIP: The Public IP of your Server (to create the exception route).
func RedirectGateway(ifaceName string, hubRealIP string) (func(), error) {
	// 1. Detect Local Gateway (e.g., 192.168.1.1)
	gw, err := GetDefaultGateway()
	if err != nil {
		return nil, fmt.Errorf("failed to detect default gateway: %v", err)
	}
	log.Printf("[ROUTE] Local Gateway detected: %s", gw)

	log.Printf("[ROUTE] Redirecting all internet traffic via %s...", ifaceName)

	// 2. Add Exception Route for Hub (Anti-Loop)
	// Command: ip route add <HUB_IP> via <GW_IP>
	// This ensures encrypted VPN packets go through the physical WiFi, not the tunnel.
	if err := runCmd("ip", "route", "add", hubRealIP, "via", gw); err != nil {
		return nil, fmt.Errorf("failed to add exception route for Hub: %v", err)
	}

	// 3. Add the 0/1 and 128/1 override routes (The "0/1 Trick")
	// These override the default gateway without deleting it.
	if err := runCmd("ip", "route", "add", "0.0.0.0/1", "dev", ifaceName); err != nil {
		// Rollback if fail
		runCmd("ip", "route", "del", hubRealIP)
		return nil, fmt.Errorf("failed to add 0/1 route: %v", err)
	}
	if err := runCmd("ip", "route", "add", "128.0.0.0/1", "dev", ifaceName); err != nil {
		// Rollback
		runCmd("ip", "route", "del", "0.0.0.0/1")
		runCmd("ip", "route", "del", hubRealIP)
		return nil, fmt.Errorf("failed to add 128/1 route: %v", err)
	}

	// 4. Return Cleanup Function
	cleanup := func() {
		log.Println("[ROUTE] Restoring default routes...")
		runCmd("ip", "route", "del", "0.0.0.0/1")
		runCmd("ip", "route", "del", "128.0.0.0/1")
		runCmd("ip", "route", "del", hubRealIP)
	}

	return cleanup, nil
}

// GetDefaultGateway parses /proc/net/route to find the current default gateway IP
func GetDefaultGateway() (string, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		// Format: Iface  Destination  Gateway  Flags ...
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		
		// Destination 00000000 means Default Gateway
		if fields[1] == "00000000" {
			// Gateway is in Hex (Little Endian). Example: 0101A8C0 -> 192.168.1.1
			gwHex := fields[2]
			if gwHex == "00000000" {
				continue // No gateway on this route
			}
			
			ip, err := parseHexIP(gwHex)
			if err != nil {
				return "", err
			}
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no default gateway found")
}

// parseHexIP converts Little Endian Hex string to net.IP
func parseHexIP(hexStr string) (net.IP, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(bytes) != 4 {
		return nil, fmt.Errorf("invalid ip length")
	}
	// Reverse bytes (Little Endian to Big Endian)
	return net.IPv4(bytes[3], bytes[2], bytes[1], bytes[0]), nil
}