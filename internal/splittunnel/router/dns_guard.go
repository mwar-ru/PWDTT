package router

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/vpn"
)

const (
	dnsPort              uint16 = 53
	defaultDNSSessionTTL        = 10 * time.Second
	maxDNSSessions              = 4096
)

// DefaultTunnelDNSv4 is used if the WireGuard adapter has no IPv4 DNS server configured.
var DefaultTunnelDNSv4 = netip.MustParseAddr("1.1.1.1")

// DNSSessionKey matches inbound UDP/53 replies from the tunnel DNS server back to the local socket.
type DNSSessionKey struct {
	LocalPort uint16
	TunnelDNS netip.Addr
}

// DNSSession stores the original 4-tuple endpoints so DNS replies are restored transparently.
type DNSSession struct {
	OrigSrcIP   netip.Addr
	OrigDstIP   netip.Addr
	OrigSrcPort uint16
	ReturnIfIdx uint32
	TunnelIP    netip.Addr
	TunnelDNS   netip.Addr
	ExpiresAt   time.Time
}

// DNSGuard intercepts system/VPN UDP/53 DNS queries in tunnel_dns mode and redirects them
// through the WireGuard tunnel without breaking browser DoH (TCP/UDP 443) or PWDTT anti-loop traffic.
type DNSGuard struct {
	mu       sync.Mutex
	sessions map[DNSSessionKey]DNSSession
	ttl      time.Duration
	now      func() time.Time
}

// NewDNSGuard creates a bounded, thread-safe DNS NAT tracker.
func NewDNSGuard() *DNSGuard {
	return &DNSGuard{
		sessions: make(map[DNSSessionKey]DNSSession, 128),
		ttl:      defaultDNSSessionTTL,
		now:      time.Now,
	}
}

// PickTunnelDNSv4 selects the first valid IPv4 DNS server configured on the tunnel adapter,
// falling back to DefaultTunnelDNSv4 (1.1.1.1).
func PickTunnelDNSv4(topo *vpn.NetworkTopology) netip.Addr {
	if topo != nil && topo.Tunnel != nil {
		for _, addr := range topo.Tunnel.DNSServers {
			if addr.Is4() && !addr.IsUnspecified() && !addr.IsLoopback() {
				return addr
			}
		}
	}
	return DefaultTunnelDNSv4
}

// ShouldInterceptOutbound decides whether an outbound packet should be steered to tunnel DNS.
func (g *DNSGuard) ShouldInterceptOutbound(
	parsed ParsedIPv4Packet,
	proc appmgr.ProcessInfo,
	policy *appmgr.PolicyEngine,
	tunnelUp bool,
) bool {
	// Only classic plaintext UDP/53 DNS; DoH (443) and DoT (853) stay untouched.
	if parsed.Proto != ProtoUDP || parsed.DstPort != dnsPort {
		return false
	}
	if !tunnelUp || policy == nil || !policy.Enabled() {
		return false
	}
	if policy.Mode() == "global" || policy.SystemDNSPolicy() != "tunnel_dns" {
		return false
	}

	// Never redirect PWDTT / wg-turn-client DNS lookups (prevents bootstrap deadlocks).
	decision := policy.Evaluate(proc)
	if decision == appmgr.DecisionBypassAntiLoop {
		return false
	}

	// Intercept Windows DNS Client service (Dnscache inside svchost.exe)
	// as well as any VPN-selected application sending direct UDP/53 queries.
	base := appmgr.BaseExeName(appmgr.NormalizeWindowsPath(proc.ExePath))
	if strings.EqualFold(base, "svchost.exe") {
		return true
	}
	return decision.UseVPN()
}

// RewriteOutbound rewrites SrcIP -> tunnel IP and DstIP -> tunnel DNS IP, recalculating checksums.
func (g *DNSGuard) RewriteOutbound(
	pkt []byte,
	parsed ParsedIPv4Packet,
	returnIfIdx uint32,
	topo *vpn.NetworkTopology,
) (uint32, bool) {
	if topo == nil || topo.Tunnel == nil || !topo.Tunnel.IPv4.Is4() || topo.Tunnel.IfIndex == 0 {
		return 0, false
	}

	tunnelIP := topo.Tunnel.IPv4
	tunnelDNS := PickTunnelDNSv4(topo)
	if !RewriteIPv4SrcDst(pkt, parsed.IHL, parsed.Proto, tunnelIP, tunnelDNS) {
		return 0, false
	}

	now := g.now()
	key := DNSSessionKey{
		LocalPort: parsed.SrcPort,
		TunnelDNS: tunnelDNS,
	}
	sess := DNSSession{
		OrigSrcIP:   parsed.SrcIP,
		OrigDstIP:   parsed.DstIP,
		OrigSrcPort: parsed.SrcPort,
		ReturnIfIdx: returnIfIdx,
		TunnelIP:    tunnelIP,
		TunnelDNS:   tunnelDNS,
		ExpiresAt:   now.Add(g.ttl),
	}

	g.mu.Lock()
	if len(g.sessions) >= maxDNSSessions {
		g.sweepLocked(now)
		if len(g.sessions) >= maxDNSSessions {
			// Evict one entry to maintain strict memory bound.
			for k := range g.sessions {
				delete(g.sessions, k)
				break
			}
		}
	}
	g.sessions[key] = sess
	g.mu.Unlock()

	return topo.Tunnel.IfIndex, true
}

// RewriteInbound restores original DNS server SrcIP and original local DstIP on UDP/53 replies.
func (g *DNSGuard) RewriteInbound(pkt []byte, parsed ParsedIPv4Packet) (uint32, bool) {
	if parsed.Proto != ProtoUDP || parsed.SrcPort != dnsPort {
		return 0, false
	}

	key := DNSSessionKey{
		LocalPort: parsed.DstPort,
		TunnelDNS: parsed.SrcIP,
	}
	now := g.now()

	g.mu.Lock()
	sess, ok := g.sessions[key]
	if ok && now.After(sess.ExpiresAt) {
		delete(g.sessions, key)
		ok = false
	}
	g.mu.Unlock()

	if !ok {
		return 0, false
	}
	if !RewriteIPv4SrcDst(pkt, parsed.IHL, parsed.Proto, sess.OrigDstIP, sess.OrigSrcIP) {
		return 0, false
	}
	return sess.ReturnIfIdx, true
}

// SweepExpired removes expired DNS NAT sessions and returns the count evicted.
func (g *DNSGuard) SweepExpired(now time.Time) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sweepLocked(now)
}

func (g *DNSGuard) sweepLocked(now time.Time) int {
	removed := 0
	for k, v := range g.sessions {
		if now.After(v.ExpiresAt) {
			delete(g.sessions, k)
			removed++
		}
	}
	return removed
}

// RunSweep periodically purges expired DNS NAT entries until ctx is cancelled.
func (g *DNSGuard) RunSweep(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			g.SweepExpired(now)
		}
	}
}

// Clear drops all active DNS NAT sessions.
func (g *DNSGuard) Clear() {
	g.mu.Lock()
	clear(g.sessions)
	g.mu.Unlock()
}

// Len returns the current number of tracked DNS sessions.
func (g *DNSGuard) Len() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.sessions)
}

// RewriteIPv4SrcDst updates both IPv4 source and destination addresses in-place
// and recalculates IPv4 + L4 checksums.
func RewriteIPv4SrcDst(pkt []byte, ihl int, proto Protocol, newSrcIP, newDstIP netip.Addr) bool {
	if !newSrcIP.Is4() || !newDstIP.Is4() || len(pkt) < ihl+8 {
		return false
	}
	src4 := newSrcIP.As4()
	dst4 := newDstIP.As4()
	copy(pkt[12:16], src4[:])
	copy(pkt[16:20], dst4[:])
	RecalculateIPv4Checksums(pkt, ihl, proto)
	return true
}
