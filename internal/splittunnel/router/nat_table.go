package router

import (
	"net/netip"
	"sync"
	"time"

	"pwdtt/internal/splittunnel/appmgr"
)

const (
	defaultMaxNATEntries = 16384
	defaultTCPTTL        = 120 * time.Second
	defaultUDPTTL        = 45 * time.Second

	ephemeralPortMin = 49152
	ephemeralPortMax = 65535
)

// NATEntry tracks bidirectional SNAT/DNAT translation state for one active flow.
type NATEntry struct {
	Proto         Protocol
	OrigLocalAddr netip.Addr
	OrigLocalPort uint16
	NATLocalAddr  netip.Addr
	NATLocalPort  uint16
	RemoteAddr    netip.Addr
	RemotePort    uint16
	PID           uint32
	Decision      appmgr.RouteDecision
	TargetIfIdx   uint32
	ReturnIfIdx   uint32
	LastSeen      time.Time
}

// NATTable is a bounded, goroutine-safe bidirectional SNAT/DNAT table for TCP and UDP/QUIC flows.
type NATTable struct {
	mu         sync.RWMutex
	outbound   map[FlowKey]*NATEntry
	inbound    map[FlowKey]*NATEntry
	maxEntries int
	tcpTTL     time.Duration
	udpTTL     time.Duration
	nextPort   uint16
	nowFn      func() time.Time
}

// NewNATTable creates a NATTable with default capacity (16,384 sessions) and TCP/UDP TTLs.
func NewNATTable() *NATTable {
	return &NATTable{
		outbound:   make(map[FlowKey]*NATEntry, 512),
		inbound:    make(map[FlowKey]*NATEntry, 512),
		maxEntries: defaultMaxNATEntries,
		tcpTTL:     defaultTCPTTL,
		udpTTL:     defaultUDPTTL,
		nextPort:   ephemeralPortMin,
		nowFn:      time.Now,
	}
}

// GetOrCreateOutbound resolves or creates a bidirectional NAT session for an outbound packet.
// Returns a copy of the NATEntry and true if newly created.
func (t *NATTable) GetOrCreateOutbound(
	orig FlowKey,
	natAddr netip.Addr,
	targetIfIdx uint32,
	returnIfIdx uint32,
	pid uint32,
	decision appmgr.RouteDecision,
) (NATEntry, bool) {
	now := t.nowFn()

	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.outbound[orig]; ok {
		existing.LastSeen = now
		return *existing, false
	}

	// Enforce bounded memory: sweep expired first, then evict oldest if still full
	if len(t.outbound) >= t.maxEntries {
		t.sweepExpiredLocked(now)
		if len(t.outbound) >= t.maxEntries {
			t.evictOldestLocked(t.maxEntries / 8)
		}
	}

	natPort := orig.LocalPort
	inKey := FlowKey{
		Proto:      orig.Proto,
		LocalAddr:  natAddr,
		LocalPort:  natPort,
		RemoteAddr: orig.RemoteAddr,
		RemotePort: orig.RemotePort,
	}

	// Resolve rare port collision on the NAT address by allocating a free ephemeral port
	if _, collides := t.inbound[inKey]; collides {
		natPort = t.allocateFreePortLocked(orig.Proto, natAddr, orig.RemoteAddr, orig.RemotePort)
		inKey.LocalPort = natPort
	}

	entry := &NATEntry{
		Proto:         orig.Proto,
		OrigLocalAddr: orig.LocalAddr,
		OrigLocalPort: orig.LocalPort,
		NATLocalAddr:  natAddr,
		NATLocalPort:  natPort,
		RemoteAddr:    orig.RemoteAddr,
		RemotePort:    orig.RemotePort,
		PID:           pid,
		Decision:      decision,
		TargetIfIdx:   targetIfIdx,
		ReturnIfIdx:   returnIfIdx,
		LastSeen:      now,
	}

	t.outbound[orig] = entry
	t.inbound[inKey] = entry
	return *entry, true
}

// LookupOutbound checks whether an outbound flow already exists in the NAT table.
func (t *NATTable) LookupOutbound(orig FlowKey) (NATEntry, bool) {
	now := t.nowFn()
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, ok := t.outbound[orig]
	if !ok {
		return NATEntry{}, false
	}
	if t.isExpired(entry, now) {
		t.deleteEntryLocked(entry)
		return NATEntry{}, false
	}
	entry.LastSeen = now
	return *entry, true
}

// LookupInbound finds the original local endpoint for an inbound reply packet (DNAT).
func (t *NATTable) LookupInbound(natFlow FlowKey) (NATEntry, bool) {
	now := t.nowFn()
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, ok := t.inbound[natFlow]
	if !ok {
		return NATEntry{}, false
	}
	if t.isExpired(entry, now) {
		t.deleteEntryLocked(entry)
		return NATEntry{}, false
	}
	entry.LastSeen = now
	return *entry, true
}

// SweepExpired removes all stale TCP and UDP/QUIC sessions and returns the removed count.
func (t *NATTable) SweepExpired(now time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sweepExpiredLocked(now)
}

func (t *NATTable) sweepExpiredLocked(now time.Time) int {
	removed := 0
	for _, entry := range t.outbound {
		if t.isExpired(entry, now) {
			t.deleteEntryLocked(entry)
			removed++
		}
	}
	return removed
}

func (t *NATTable) isExpired(entry *NATEntry, now time.Time) bool {
	ttl := t.udpTTL
	if entry.Proto == ProtoTCP {
		ttl = t.tcpTTL
	}
	return now.Sub(entry.LastSeen) > ttl
}

func (t *NATTable) deleteEntryLocked(entry *NATEntry) {
	outKey := FlowKey{
		Proto:      entry.Proto,
		LocalAddr:  entry.OrigLocalAddr,
		LocalPort:  entry.OrigLocalPort,
		RemoteAddr: entry.RemoteAddr,
		RemotePort: entry.RemotePort,
	}
	inKey := FlowKey{
		Proto:      entry.Proto,
		LocalAddr:  entry.NATLocalAddr,
		LocalPort:  entry.NATLocalPort,
		RemoteAddr: entry.RemoteAddr,
		RemotePort: entry.RemotePort,
	}
	delete(t.outbound, outKey)
	delete(t.inbound, inKey)
}

func (t *NATTable) evictOldestLocked(count int) {
	if count < 1 {
		count = 1
	}
	for i := 0; i < count && len(t.outbound) > 0; i++ {
		var oldest *NATEntry
		for _, entry := range t.outbound {
			if oldest == nil || entry.LastSeen.Before(oldest.LastSeen) {
				oldest = entry
			}
		}
		if oldest == nil {
			break
		}
		t.deleteEntryLocked(oldest)
	}
}

func (t *NATTable) allocateFreePortLocked(proto Protocol, natAddr, remoteAddr netip.Addr, remotePort uint16) uint16 {
	span := ephemeralPortMax - ephemeralPortMin + 1
	for i := 0; i < span; i++ {
		candidate := t.nextPort
		if t.nextPort >= ephemeralPortMax {
			t.nextPort = ephemeralPortMin
		} else {
			t.nextPort++
		}
		probe := FlowKey{
			Proto:      proto,
			LocalAddr:  natAddr,
			LocalPort:  candidate,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
		}
		if _, used := t.inbound[probe]; !used {
			return candidate
		}
	}
	return t.nextPort
}

// Clear drops all active NAT sessions (e.g. on tunnel reconnect or Wi-Fi <-> Ethernet switch).
func (t *NATTable) Clear() {
	t.mu.Lock()
	t.outbound = make(map[FlowKey]*NATEntry, 512)
	t.inbound = make(map[FlowKey]*NATEntry, 512)
	t.mu.Unlock()
}

// Len returns the number of active NAT sessions.
func (t *NATTable) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.outbound)
}
