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
    if err := runCmd("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
        return nil, fmt.Errorf("failed to enable ip_forward: %v", err)
    }

    // 2. Configure IPTables (NAT)
    // SOLUCIÓN CLAVE: Usamos "-I" (Insert) en lugar de "-A" (Append) 
    // para asegurarnos de que nuestras reglas tengan prioridad sobre Docker/UFW.

    // Rule A: Masquerade outgoing traffic (The "Lie")
    // Target: Traffic leaving physical interfaces (NOT tun0)
    if err := runCmd("iptables", "-t", "nat", "-I", "POSTROUTING", "1", "!", "-o", tunName, "-j", "MASQUERADE"); err != nil {
        return nil, fmt.Errorf("failed to set MASQUERADE rule: %v", err)
    }

    // Rule B: Allow Forwarding traffic originating from TUN
    // "Permitir que lo que entra por el túnel salga a internet"
    if err := runCmd("iptables", "-I", "FORWARD", "1", "-i", tunName, "-j", "ACCEPT"); err != nil {
        return nil, fmt.Errorf("failed to allow FORWARD in: %v", err)
    }

    // Rule C: Allow Forwarding traffic returning to TUN
    // "Permitir que la respuesta de internet vuelva a entrar al túnel"
    if err := runCmd("iptables", "-I", "FORWARD", "1", "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
        return nil, fmt.Errorf("failed to allow FORWARD return: %v", err)
    }

    log.Println("[NAT] Network Address Translation (NAT) enabled.")

    // 3. Return Cleanup Closure
    // Usamos -D (Delete) para limpiar exactamente lo que creamos
    cleanup := func() {
        log.Println("[NAT] Cleaning up iptables rules...")
        // Nota: Al borrar, no hace falta especificar el número "1", iptables busca por coincidencia.
        runCmd("iptables", "-t", "nat", "-D", "POSTROUTING", "!", "-o", tunName, "-j", "MASQUERADE")
        runCmd("iptables", "-D", "FORWARD", "-i", tunName, "-j", "ACCEPT")
        runCmd("iptables", "-D", "FORWARD", "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
    }

    return cleanup, nil
}