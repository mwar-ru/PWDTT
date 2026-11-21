package router

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"testing"
	"time"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

func buildTestUDPPacket(srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) []byte {
	totalLen := 20 + 8 + len(payload)
	pkt := make([]byte, totalLen)
	pkt[0] = 0x45 // IPv4, IHL=20
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	pkt[8] = 64 // TTL
	pkt[9] = byte(ProtoUDP)
	s := netip.MustParseAddr(srcIP).As4()
	d := netip.MustParseAddr(dstIP).As4()
	copy(pkt[12:16], s[:])
	copy(pkt[16:20], d[:])

	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(8+len(payload)))
	copy(pkt[28:], payload)

	RecalculateIPv4Checksums(pkt, 20, ProtoUDP)
	return pkt
}

func TestDNSGuardInterceptPolicy(t *testing.T) {
	guard := NewDNSGuard()
	cfg := settings.DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = "include"
	cfg.SystemDNS = "tunnel_dns"
	cfg.Apps = []settings.AppConfig{
		{ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`, MatchChildren: true},
	}
	policy := appmgr.NewPolicyEngine(cfg, `C:\Program Files\PWDTT\pwdtt.exe`)

	udp53 := ParsedIPv4Packet{
		IHL:     20,
		Proto:   ProtoUDP,
		SrcIP:   netip.MustParseAddr("192.168.1.50"),
		DstIP:   netip.MustParseAddr("8.8.8.8"),
		SrcPort: 54321,
		DstPort: 53,
	}
	doh443 := ParsedIPv4Packet{
		IHL:     20,
		Proto:   ProtoUDP,
		SrcIP:   netip.MustParseAddr("192.168.1.50"),
		DstIP:   netip.MustParseAddr("1.1.1.1"),
		SrcPort: 54322,
		DstPort: 443,
	}
	tcp53 := udp53
	tcp53.Proto = ProtoTCP

	svchost := appmgr.ProcessInfo{PID: 100, ExePath: `C:\Windows\System32\svchost.exe`}
	pwdtt := appmgr.ProcessInfo{PID: 200, ExePath: `C:\Program Files\PWDTT\pwdtt.exe`}
	wgTurn := appmgr.ProcessInfo{PID: 201, ExePath: `C:\Program Files\PWDTT\wg-turn-client.exe`}
	firefox := appmgr.ProcessInfo{PID: 300, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`}
	chrome := appmgr.ProcessInfo{PID: 400, ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`}

	if !guard.ShouldInterceptOutbound(udp53, svchost, policy, true) {
		t.Fatal("expected svchost.exe UDP/53 to be intercepted in tunnel_dns mode")
	}
	if !guard.ShouldInterceptOutbound(udp53, firefox, policy, true) {
		t.Fatal("expected VPN-selected app direct UDP/53 to be intercepted")
	}
	if guard.ShouldInterceptOutbound(udp53, chrome, policy, true) {
		t.Fatal("expected unselected app direct UDP/53 to remain untouched in include mode")
	}
	if guard.ShouldInterceptOutbound(udp53, pwdtt, policy, true) {
		t.Fatal("anti-loop violation: pwdtt.exe DNS must never be redirected")
	}
	if guard.ShouldInterceptOutbound(udp53, wgTurn, policy, true) {
		t.Fatal("anti-loop violation: wg-turn-client.exe DNS must never be redirected")
	}
	if guard.ShouldInterceptOutbound(doh443, firefox, policy, true) {
		t.Fatal("DoH port 443 must remain untouched by DNS Guard")
	}
	if guard.ShouldInterceptOutbound(tcp53, svchost, policy, true) {
		t.Fatal("TCP/53 should not be handled by UDP/53 DNS Guard hook")
	}
	if guard.ShouldInterceptOutbound(udp53, svchost, policy, false) {
		t.Fatal("must not redirect DNS when tunnel is down")
	}

	cfgSystemDNS := cfg
	cfgSystemDNS.SystemDNS = "system_dns"
	policySystem := appmgr.NewPolicyEngine(cfgSystemDNS, `C:\Program Files\PWDTT\pwdtt.exe`)
	if guard.ShouldInterceptOutbound(udp53, svchost, policySystem, true) {
		t.Fatal("must not intercept when system_dns policy is selected")
	}

	cfgGlobal := cfg
	cfgGlobal.Mode = "global"
	policyGlobal := appmgr.NewPolicyEngine(cfgGlobal, `C:\Program Files\PWDTT\pwdtt.exe`)
	if guard.ShouldInterceptOutbound(udp53, svchost, policyGlobal, true) {
		t.Fatal("must not intercept in global mode")
	}
}

func TestDNSGuardOutboundInboundRewriteAndChecksums(t *testing.T) {
	guard := NewDNSGuard()
	topo := &vpn.NetworkTopology{
		Tunnel: &vpn.InterfaceInfo{
			IfIndex:    42,
			IPv4:       netip.MustParseAddr("10.8.0.2"),
			DNSServers: []netip.Addr{netip.MustParseAddr("2606:4700:4700::1111"), netip.MustParseAddr("10.8.0.1")},
		},
	}

	queryPkt := buildTestUDPPacket("192.168.1.50", "8.8.8.8", 55123, 53, []byte{0xde, 0xad, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	parsedOut, ok := ParseIPv4Transport(queryPkt)
	if !ok {
		t.Fatal("failed to parse test DNS query")
	}

	targetIf, ok := guard.RewriteOutbound(queryPkt, parsedOut, 7, topo)
	if !ok {
		t.Fatal("RewriteOutbound failed")
	}
	if targetIf != 42 {
		t.Fatalf("expected tunnel IfIndex 42, got %d", targetIf)
	}
	if !VerifyIPv4HeaderChecksum(queryPkt) || !VerifyIPv4L4Checksum(queryPkt) {
		t.Fatal("rewritten outbound DNS packet has invalid checksum")
	}

	rewrittenOut, _ := ParseIPv4Transport(queryPkt)
	if rewrittenOut.SrcIP != netip.MustParseAddr("10.8.0.2") || rewrittenOut.DstIP != netip.MustParseAddr("10.8.0.1") {
		t.Fatalf("unexpected rewritten outbound IPs: %s -> %s", rewrittenOut.SrcIP, rewrittenOut.DstIP)
	}

	replyPkt := buildTestUDPPacket("10.8.0.1", "10.8.0.2", 53, 55123, []byte{0xde, 0xad, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00})
	parsedIn, ok := ParseIPv4Transport(replyPkt)
	if !ok {
		t.Fatal("failed to parse test DNS reply")
	}

	returnIf, ok := guard.RewriteInbound(replyPkt, parsedIn)
	if !ok {
		t.Fatal("RewriteInbound failed to match DNS session")
	}
	if returnIf != 7 {
		t.Fatalf("expected returnIfIdx 7, got %d", returnIf)
	}
	if !VerifyIPv4HeaderChecksum(replyPkt) || !VerifyIPv4L4Checksum(replyPkt) {
		t.Fatal("restored inbound DNS reply has invalid checksum")
	}

	restoredIn, _ := ParseIPv4Transport(replyPkt)
	if restoredIn.SrcIP != netip.MustParseAddr("8.8.8.8") || restoredIn.DstIP != netip.MustParseAddr("192.168.1.50") {
		t.Fatalf("unexpected restored inbound IPs: %s -> %s", restoredIn.SrcIP, restoredIn.DstIP)
	}
}

func TestDNSGuardPickTunnelDNSFallback(t *testing.T) {
	if got := PickTunnelDNSv4(nil); got != DefaultTunnelDNSv4 {
		t.Fatalf("expected fallback %s, got %s", DefaultTunnelDNSv4, got)
	}
	topoV6Only := &vpn.NetworkTopology{
		Tunnel: &vpn.InterfaceInfo{
			DNSServers: []netip.Addr{netip.MustParseAddr("2606:4700:4700::1111")},
		},
	}
	if got := PickTunnelDNSv4(topoV6Only); got != DefaultTunnelDNSv4 {
		t.Fatalf("expected fallback %s for IPv6-only DNS list, got %s", DefaultTunnelDNSv4, got)
	}
}

func TestDNSGuardExpiryAndConcurrentRace(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	guard := NewDNSGuard()
	guard.ttl = 2 * time.Second
	guard.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}

	topo := &vpn.NetworkTopology{
		Tunnel: &vpn.InterfaceInfo{
			IfIndex:    9,
			IPv4:       netip.MustParseAddr("10.8.0.2"),
			DNSServers: []netip.Addr{netip.MustParseAddr("1.1.1.1")},
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		port := uint16(40000 + i)
		wg.Add(1)
		go func(p uint16) {
			defer wg.Done()
			q := buildTestUDPPacket("192.168.1.10", "8.8.4.4", p, 53, []byte{1, 2, 3, 4})
			parsedQ, _ := ParseIPv4Transport(q)
			if _, ok := guard.RewriteOutbound(q, parsedQ, 3, topo); !ok {
				t.Errorf("RewriteOutbound failed for port %d", p)
				return
			}
			r := buildTestUDPPacket("1.1.1.1", "10.8.0.2", 53, p, []byte{1, 2, 3, 4})
			parsedR, _ := ParseIPv4Transport(r)
			if _, ok := guard.RewriteInbound(r, parsedR); !ok {
				t.Errorf("RewriteInbound failed for port %d", p)
			}
		}(port)
	}
	wg.Wait()

	if guard.Len() != 32 {
		t.Fatalf("expected 32 DNS sessions, got %d", guard.Len())
	}

	mu.Lock()
	now = now.Add(3 * time.Second)
	sweepTime := now
	mu.Unlock()

	if evicted := guard.SweepExpired(sweepTime); evicted != 32 {
		t.Fatalf("expected 32 expired sessions, got %d", evicted)
	}
	if guard.Len() != 0 {
		t.Fatalf("expected 0 sessions after sweep, got %d", guard.Len())
	}
}
