package tun

import (
	"fmt"
	"log"
)

// EnableExitNode configures the OS to act as a NAT router.
// 1. Enables IP Forwarding (sysctl).
// 2. Adds IPTables rules for Masquerading (NAT).
// 3. Returns a cleanup function to revert changes on exit.
func EnableExitNode(tunName string) (func(), error) {
	log.Printf("[NAT] Enabling Exit Node features on %s...", tunName)

	// 1. Enable IP Forwarding
	// Equivalent to: sysctl -w net.ipv4.ip_forward=1
	if err := runCmd("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return nil, fmt.Errorf("failed to enable ip_forward: %v", err)
	}

	// 2. Configure IPTables (NAT)
	// We detect the default physical interface (e.g., eth0, wlan0) automatically
	// strictly speaking, MASQUERADE handles dynamic IPs automatically.
	
	// Rule A: Masquerade outgoing traffic (The "Lie")
	// iptables -t nat -A POSTROUTING -o <physical> -j MASQUERADE
	// Ideally, we let Linux handle the interface selection by not specifying -o, 
	// or we accept specific interface via config. 
	// For simplicity and robustness, we target traffic NOT coming from tun0.
	
	// Command: iptables -t nat -A POSTROUTING ! -o tun0 -j MASQUERADE
	if err := runCmd("iptables", "-t", "nat", "-A", "POSTROUTING", "!", "-o", tunName, "-j", "MASQUERADE"); err != nil {
		return nil, fmt.Errorf("failed to set MASQUERADE rule: %v", err)
	}

	// Rule B: Allow Forwarding from TUN
	// iptables -A FORWARD -i tun0 -j ACCEPT
	if err := runCmd("iptables", "-A", "FORWARD", "-i", tunName, "-j", "ACCEPT"); err != nil {
		return nil, fmt.Errorf("failed to allow FORWARD in: %v", err)
	}

	// Rule C: Allow Forwarding back to TUN (Established connections)
	// iptables -A FORWARD -o tun0 -m state --state RELATED,ESTABLISHED -j ACCEPT
	if err := runCmd("iptables", "-A", "FORWARD", "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return nil, fmt.Errorf("failed to allow FORWARD return: %v", err)
	}

	log.Println("[NAT] Network Address Translation (NAT) enabled.")

	// 3. Return Cleanup Closure
	// This function will be called when the program stops (defer)
	cleanup := func() {
		log.Println("[NAT] Cleaning up iptables rules...")
		// Delete (-D) the rules we added (-A)
		runCmd("iptables", "-t", "nat", "-D", "POSTROUTING", "!", "-o", tunName, "-j", "MASQUERADE")
		runCmd("iptables", "-D", "FORWARD", "-i", tunName, "-j", "ACCEPT")
		runCmd("iptables", "-D", "FORWARD", "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	}

	return cleanup, nil
}