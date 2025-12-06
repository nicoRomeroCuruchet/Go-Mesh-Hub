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
	routes map[string]*PeerStats // Changed value from *net.UDPAddr to *PeerStats
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