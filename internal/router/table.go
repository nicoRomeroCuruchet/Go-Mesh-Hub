package router

import (
	"log"
	"net"
	"sync"
	"time"
)

// PeerStats holds the state of a connected client
type PeerStats struct {
	VirtualIP string
	RealAddr  string
	LastSeen  time.Time
	RxBytes   uint64
	TxBytes   uint64
}

// Table manages the mapping between Virtual IPs and Peer Data
type Table struct {
	sync.RWMutex
	routes map[string]*PeerStats
	exitNodeIP string
}

func NewTable() *Table {
	return &Table{
		routes: make(map[string]*PeerStats),
	}
}

// Learn updates the route and refreshes "LastSeen"
// It returns the PeerStats object so we can update RxBytes externally if needed
func (t *Table) Learn(virtualIP string, realAddr *net.UDPAddr) {
	t.Lock()
	defer t.Unlock()

	peer, exists := t.routes[virtualIP]
	if !exists {
		peer = &PeerStats{VirtualIP: virtualIP}
		t.routes[virtualIP] = peer
		log.Printf("[ROUTE] New Peer Learned: %s at %s", virtualIP, realAddr)
	}

	// Update dynamic fields
	if peer.RealAddr != realAddr.String() {
		log.Printf("[ROUTE] Peer %s moved to %s", virtualIP, realAddr)
		peer.RealAddr = realAddr.String()
	}
	peer.LastSeen = time.Now()
}

// Lookup finds the Real UDP Address for a given Virtual IP
func (t *Table) Lookup(virtualIP string) *net.UDPAddr {
	t.RLock()
	defer t.RUnlock()

	if peer, ok := t.routes[virtualIP]; ok {
		// Convert string back to UDPAddr (cached or parsed)
		addr, _ := net.ResolveUDPAddr("udp", peer.RealAddr)
		return addr
	}
	return nil
}

// RecordRx increments the Received Bytes counter for a peer
func (t *Table) RecordRx(virtualIP string, bytes int) {
	t.Lock()
	defer t.Unlock()
	if peer, ok := t.routes[virtualIP]; ok {
		peer.RxBytes += uint64(bytes)
	}
}

// RecordTx increments the Transmitted Bytes counter for a peer
func (t *Table) RecordTx(virtualIP string, bytes int) {
	t.Lock()
	defer t.Unlock()
	if peer, ok := t.routes[virtualIP]; ok {
		peer.TxBytes += uint64(bytes)
	}
}

// Snapshot returns a copy of all peers for the Dashboard (Thread-Safe)
func (t *Table) Snapshot() []PeerStats {
	t.RLock()
	defer t.RUnlock()

	peers := make([]PeerStats, 0, len(t.routes))
	for _, p := range t.routes {
		// Return a copy, not a pointer, to prevent race conditions in UI rendering
		peers = append(peers, *p)
	}
	return peers
}

// SetExitNode defines which Virtual IP acts as the default gateway for internet traffic
func (t *Table) SetExitNode(virtualIP string) {
    t.Lock()
    defer t.Unlock()
    t.exitNodeIP = virtualIP
    log.Printf("[ROUTER] Exit Node set to: %s", virtualIP)
}

// GetRoute decides where to send the packet based on Destination IP.
// This implements the core "Split Tunneling" vs "Full Tunneling" logic support.
func (t *Table) GetRoute(dstIP string) (*net.UDPAddr, bool) {
    t.RLock()
    defer t.RUnlock()

    // 1. Direct Peer Match (VPN Mesh Traffic)
    // Example: 10.0.0.2 talking to 10.0.0.3
    if peer, ok := t.routes[dstIP]; ok {
        // Resolve stored address string back to UDPAddr
        addr, _ := net.ResolveUDPAddr("udp", peer.RealAddr)
        return addr, true
    }

    // 2. Default Route (Internet Traffic via Exit Node)
    // If destination is NOT a peer (e.g. 8.8.8.8), and we have an Exit Node configured...
    if t.exitNodeIP != "" {
        // We look up the Real Address of the Exit Node itself
        if exitPeer, ok := t.routes[t.exitNodeIP]; ok {
            addr, _ := net.ResolveUDPAddr("udp", exitPeer.RealAddr)
            return addr, true
        }
    }

    // 3. No Route Found (Drop packet)
    return nil, false
}