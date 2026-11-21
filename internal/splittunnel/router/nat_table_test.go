package router

import (
	"net/netip"
	"sync"
	"testing"
	"time"

	"pwdtt/internal/splittunnel/appmgr"
)

func TestNATTableOutboundInboundRoundtripAndCollision(t *testing.T) {
	tbl := NewNATTable()
	physIP := netip.MustParseAddr("192.168.1.42")
	physIP2 := netip.MustParseAddr("192.168.1.43")
	tunIP := netip.MustParseAddr("10.66.66.2")
	remoteIP := netip.MustParseAddr("1.1.1.1")

	origFlow := FlowKey{
		Proto:      ProtoTCP,
		LocalAddr:  physIP,
		LocalPort:  54321,
		RemoteAddr: remoteIP,
		RemotePort: 443,
	}

	entry, created := tbl.GetOrCreateOutbound(origFlow, tunIP, 22, 10, 4040, appmgr.DecisionVPN)
	if !created {
		t.Fatalf("expected new NAT entry")
	}
	if entry.NATLocalAddr != tunIP || entry.NATLocalPort != 54321 {
		t.Fatalf("unexpected NAT mapping: %+v", entry)
	}

	// Inbound reply arrives on Wintun targeting tunIP:54321 from remoteIP:443
	replyKey := FlowKey{
		Proto:      ProtoTCP,
		LocalAddr:  tunIP,
		LocalPort:  entry.NATLocalPort,
		RemoteAddr: remoteIP,
		RemotePort: 443,
	}
	found, ok := tbl.LookupInbound(replyKey)
	if !ok {
		t.Fatalf("expected inbound DNAT hit")
	}
	if found.OrigLocalAddr != physIP || found.OrigLocalPort != 54321 || found.ReturnIfIdx != 10 {
		t.Fatalf("unexpected DNAT entry: %+v", found)
	}

	// Collision: another local IP uses the exact same source port 54321 to same remote
	collidingFlow := FlowKey{
		Proto:      ProtoTCP,
		LocalAddr:  physIP2,
		LocalPort:  54321,
		RemoteAddr: remoteIP,
		RemotePort: 443,
	}
	entry2, created2 := tbl.GetOrCreateOutbound(collidingFlow, tunIP, 22, 10, 5050, appmgr.DecisionVPN)
	if !created2 {
		t.Fatalf("expected new NAT entry for colliding flow")
	}
	if entry2.NATLocalPort == entry.NATLocalPort {
		t.Fatalf("expected distinct NATLocalPort on collision, both got %d", entry2.NATLocalPort)
	}
}

func TestNATTableSeparateTCPAndUDPQUICTTLs(t *testing.T) {
	now := time.Now()
	tbl := NewNATTable()
	tbl.nowFn = func() time.Time { return now }

	physIP := netip.MustParseAddr("192.168.1.42")
	tunIP := netip.MustParseAddr("10.66.66.2")
	remoteIP := netip.MustParseAddr("142.250.74.14")

	tcpFlow := FlowKey{Proto: ProtoTCP, LocalAddr: physIP, LocalPort: 50001, RemoteAddr: remoteIP, RemotePort: 443}
	quicFlow := FlowKey{Proto: ProtoUDP, LocalAddr: physIP, LocalPort: 50002, RemoteAddr: remoteIP, RemotePort: 443}

	tbl.GetOrCreateOutbound(tcpFlow, tunIP, 22, 10, 100, appmgr.DecisionVPN)
	tbl.GetOrCreateOutbound(quicFlow, tunIP, 22, 10, 100, appmgr.DecisionVPN)

	// Advance 50s -> UDP/QUIC (45s TTL) MUST expire, TCP (120s TTL) MUST survive
	now = now.Add(50 * time.Second)
	removed := tbl.SweepExpired(now)
	if removed != 1 {
		t.Fatalf("expected 1 expired UDP/QUIC session, removed %d", removed)
	}
	if _, ok := tbl.LookupOutbound(quicFlow); ok {
		t.Fatalf("expected QUIC flow expired")
	}
	if _, ok := tbl.LookupOutbound(tcpFlow); !ok {
		t.Fatalf("expected TCP flow still alive at 50s")
	}

	// Advance another 125s -> TCP expires too
	now = now.Add(125 * time.Second)
	if removed2 := tbl.SweepExpired(now); removed2 != 1 || tbl.Len() != 0 {
		t.Fatalf("expected TCP expired and table empty, removed=%d len=%d", removed2, tbl.Len())
	}
}

func TestNATTableBoundedCapacity(t *testing.T) {
	tbl := NewNATTable()
	tbl.maxEntries = 16
	physIP := netip.MustParseAddr("192.168.1.10")
	tunIP := netip.MustParseAddr("10.66.66.2")
	remoteIP := netip.MustParseAddr("93.184.216.34")

	for i := uint16(1000); i < 1050; i++ {
		tbl.GetOrCreateOutbound(FlowKey{
			Proto: ProtoUDP, LocalAddr: physIP, LocalPort: i, RemoteAddr: remoteIP, RemotePort: 443,
		}, tunIP, 22, 10, 1, appmgr.DecisionVPN)
	}
	if tbl.Len() > tbl.maxEntries {
		t.Fatalf("NATTable exceeded maxEntries: %d > %d", tbl.Len(), tbl.maxEntries)
	}
}

func TestStagingQueueFIFODrainExpiryAndCaps(t *testing.T) {
	now := time.Now()
	q := NewStagingQueue()
	q.nowFn = func() time.Time { return now }

	keyA := SocketKey{Proto: ProtoUDP, LocalPort: 51000}
	keyB := SocketKey{Proto: ProtoTCP, LocalPort: 52000}

	if !q.Enqueue(StagedPacket{Key: keyA, RawPacket: []byte{1, 2, 3}}) {
		t.Fatalf("enqueue 1 failed")
	}
	if !q.Enqueue(StagedPacket{Key: keyA, RawPacket: []byte{4, 5, 6}}) {
		t.Fatalf("enqueue 2 failed")
	}
	if !q.Enqueue(StagedPacket{Key: keyB, RawPacket: []byte{9, 9}}) {
		t.Fatalf("enqueue keyB failed")
	}

	drained := q.DrainForSocket(keyA)
	if len(drained) != 2 || drained[0].RawPacket[0] != 1 || drained[1].RawPacket[0] != 4 {
		t.Fatalf("expected 2 FIFO packets for keyA, got %+v", drained)
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 packet remaining (keyB), got %d", q.Len())
	}

	// Advance 30ms (> 25ms hold timeout) -> keyB expires
	now = now.Add(30 * time.Millisecond)
	expired := q.PopExpired(now)
	if len(expired) != 1 || expired[0].Key != keyB {
		t.Fatalf("expected keyB expired, got %+v", expired)
	}
	if q.Len() != 0 {
		t.Fatalf("expected empty staging queue, got %d", q.Len())
	}

	// Verify per-key cap (defaultStagingMaxPerKey = 8)
	for i := 0; i < defaultStagingMaxPerKey; i++ {
		if !q.Enqueue(StagedPacket{Key: keyA, RawPacket: []byte{byte(i)}}) {
			t.Fatalf("unexpected enqueue reject at %d", i)
		}
	}
	if q.Enqueue(StagedPacket{Key: keyA, RawPacket: []byte{99}}) {
		t.Fatalf("expected per-key cap to reject 9th packet")
	}
}

func TestNATTableAndStagingQueueConcurrentRace(t *testing.T) {
	tbl := NewNATTable()
	q := NewStagingQueue()
	physIP := netip.MustParseAddr("192.168.1.42")
	tunIP := netip.MustParseAddr("10.66.66.2")
	remoteIP := netip.MustParseAddr("1.1.1.1")

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			port := uint16(40000 + id)
			sk := SocketKey{Proto: ProtoUDP, LocalPort: port}
			fk := FlowKey{Proto: ProtoUDP, LocalAddr: physIP, LocalPort: port, RemoteAddr: remoteIP, RemotePort: 443}
			for j := 0; j < 100; j++ {
				q.Enqueue(StagedPacket{Key: sk, RawPacket: []byte{byte(j)}})
				entry, _ := tbl.GetOrCreateOutbound(fk, tunIP, 22, 10, uint32(id), appmgr.DecisionVPN)
				tbl.LookupInbound(FlowKey{
					Proto: ProtoUDP, LocalAddr: tunIP, LocalPort: entry.NATLocalPort, RemoteAddr: remoteIP, RemotePort: 443,
				})
				if j%10 == 0 {
					q.DrainForSocket(sk)
					q.PopExpired(time.Now().Add(time.Second))
					tbl.SweepExpired(time.Now())
				}
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkNATTableLookup(b *testing.B) {
	tbl := NewNATTable()
	physIP := netip.MustParseAddr("192.168.1.42")
	tunIP := netip.MustParseAddr("10.66.66.2")
	remoteIP := netip.MustParseAddr("1.1.1.1")
	fk := FlowKey{Proto: ProtoTCP, LocalAddr: physIP, LocalPort: 54321, RemoteAddr: remoteIP, RemotePort: 443}
	entry, _ := tbl.GetOrCreateOutbound(fk, tunIP, 22, 10, 1234, appmgr.DecisionVPN)
	inKey := FlowKey{Proto: ProtoTCP, LocalAddr: tunIP, LocalPort: entry.NATLocalPort, RemoteAddr: remoteIP, RemotePort: 443}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tbl.LookupInbound(inKey)
	}
}
