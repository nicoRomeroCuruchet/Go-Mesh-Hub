package tun

import (
	"fmt"
	"log"
	"os/exec"
)

// IPTables Constants to avoid "magic strings" and typos.
const (
	TableNat    = "nat"
	TableFilter = "filter"

	ChainPostRouting = "POSTROUTING"
	ChainForward     = "FORWARD"

	TargetMasquerade = "MASQUERADE"
	TargetAccept     = "ACCEPT"
)

// Rule represents a single iptables rule configuration.
// We use a struct to ensure consistency between creation and deletion.
type Rule struct {
	Name  string   // Human-readable description for logs
	Table string   // e.g., "nat"
	Chain string   // e.g., "POSTROUTING"
	Args  []string // The specific matchers (e.g., "-o", "tun0", "-j", "MASQUERADE")
}

// EnableExitNode configures the Linux Kernel to act as a Router/NAT Gateway.
// It applies necessary sysctl configurations and iptables rules.
// Returns a cleanup function to revert changes upon shutdown.
func EnableExitNode(tunName string) (func(), error) {
	log.Printf("[NAT] Initializing Exit Node logic on interface: %s", tunName)

	// 1. Enable IP Forwarding (Kernel Level)
	// Without this, the Linux kernel drops packets not destined for itself.
	if err := runCmd("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return nil, fmt.Errorf("failed to enable ip_forward: %w", err)
	}

	// 2. Define the Ruleset
	// We define them here to ensure the exact same arguments are used for Add and Delete.
	// The "! -o tunName" logic ensures we masquerade traffic going out to physical interfaces (eth0/wlan0).
	rules := []Rule{
		{
			Name:  "Masquerade Outbound Traffic",
			Table: TableNat,
			Chain: ChainPostRouting,
			Args:  []string{"!", "-o", tunName, "-j", TargetMasquerade},
		},
		{
			Name:  "Allow Forwarding FROM Tunnel",
			Table: TableFilter,
			Chain: ChainForward,
			Args:  []string{"-i", tunName, "-j", TargetAccept},
		},
		{
			Name:  "Allow Forwarding TO Tunnel (Established)",
			Table: TableFilter,
			Chain: ChainForward,
			Args:  []string{"-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", TargetAccept},
		},
	}

	// 3. Apply Rules (Idempotent)
	// We iterate through the definition list and ensure they exist.
	for _, rule := range rules {
		if err := ensureRule(rule); err != nil {
			return nil, fmt.Errorf("failed to apply rule '%s': %w", rule.Name, err)
		}
	}

	log.Println("[NAT] Exit Node Active: NAT and Forwarding rules applied.")

	// 4. Construct Cleanup Closure
	// This function captures the 'rules' slice via closure and reverses the operations.
	cleanup := func() {
		log.Println("[NAT] Shutdown sequence: cleaning up iptables rules...")
		for _, rule := range rules {
			if err := deleteRule(rule); err != nil {
				// We log but do not fail here, as we want to attempt clearing all rules.
				log.Printf("[NAT-ERR] Failed to cleanup rule '%s': %v", rule.Name, err)
			}
		}
	}

	return cleanup, nil
}

// ensureRule checks if a rule exists. If not, it inserts it at the top (Position 1).
func ensureRule(r Rule) error {
	// Step A: Check if rule exists (-C)
	// iptables returns exit code 0 if found, 1 if not found.
	checkArgs := append([]string{"-t", r.Table, "-C", r.Chain}, r.Args...)
	cmdCheck := exec.Command("iptables", checkArgs...)
	
	if err := cmdCheck.Run(); err == nil {
		// Rule already exists. No action needed.
		// log.Printf("[NAT-DEBUG] Rule '%s' already exists. Skipping.", r.Name)
		return nil
	}

	// Step B: Insert rule at Position 1 (-I)
	// We use Insert to ensure our rules take precedence over Docker or UFW rules.
	insertArgs := append([]string{"-t", r.Table, "-I", r.Chain, "1"}, r.Args...)
	if err := runCmd("iptables", insertArgs...); err != nil {
		return err
	}

	log.Printf("[NAT] Applied rule: %s", r.Name)
	return nil
}

// deleteRule removes the exact rule specification from the chain.
func deleteRule(r Rule) error {
	// Delete (-D) requires the exact same arguments as Creation.
	deleteArgs := append([]string{"-t", r.Table, "-D", r.Chain}, r.Args...)
	
	if err := runCmd("iptables", deleteArgs...); err != nil {
		// If the error suggests the rule doesn't exist, we can ignore it during cleanup.
		// However, standard runCmd doesn't return stdout, so we assume generic error.
		return err
	}
	
	log.Printf("[NAT] Removed rule: %s", r.Name)
	return nil
}