//go:build windows

package router

import (
	"context"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/vpn"
)

const (
	networkRecvBufSize   = 65535
	// Exclude loopback and already-reinjected impostor packets in kernel driver to prevent packet loops.
	networkFilterExpr    = "!loopback and !impostor and ip and (tcp or udp)"
	stagingSweepInterval = 10 * time.Millisecond
	natSweepInterval     = 15 * time.Second
)

// PacketHook allows optional specialized handlers (such as DNS Guard in T11) to intercept packets
// before standard per-PID NAT processing. Return handled=true if the hook already sent or dropped pkt.
type PacketHook func(pkt []byte, parsed ParsedIPv4Packet, addr *winDivertAddress, topo *vpn.NetworkTopology, policy *appmgr.PolicyEngine) (handled bool)

type networkEngine struct {
	dll      *WinDivertDLL
	tracker  *SocketTracker
	resolver *appmgr.ProcessResolver
	nat      *NATTable
	staging  *StagingQueue

	stateMu  sync.RWMutex
	policy   *appmgr.PolicyEngine
	topology *vpn.NetworkTopology
	tunnelUp bool
	dnsHook  PacketHook

	handleMu sync.Mutex
	handle   uintptr
	doneCh   chan struct{}

	packetsToVPN    atomic.Uint64
	packetsDirect   atomic.Uint64
	packetsDropped  atomic.Uint64
	dnsRedirected   atomic.Uint64
	stagingQueued   atomic.Uint64
	stagingExpired  atomic.Uint64
}

func newNetworkEngine(
	dll *WinDivertDLL,
	tracker *SocketTracker,
	resolver *appmgr.ProcessResolver,
	nat *NATTable,
	staging *StagingQueue,
) *networkEngine {
	return &networkEngine{
		dll:      dll,
		tracker:  tracker,
		resolver: resolver,
		nat:      nat,
		staging:  staging,
	}
}

func (e *networkEngine) SetPolicy(p *appmgr.PolicyEngine) {
	e.stateMu.Lock()
	e.policy = p
	e.stateMu.Unlock()
}

func (e *networkEngine) SetTopology(topo *vpn.NetworkTopology, tunnelUp bool) {
	e.stateMu.Lock()
	if topo != nil {
		cp := *topo
		e.topology = &cp
	} else {
		e.topology = nil
	}
	e.tunnelUp = tunnelUp
	e.stateMu.Unlock()
}

func (e *networkEngine) SetDNSHook(hook PacketHook) {
	e.stateMu.Lock()
	e.dnsHook = hook
	e.stateMu.Unlock()
}

func (e *networkEngine) Start(ctx context.Context) error {
	h, err := e.dll.Open(networkFilterExpr, winDivertLayerNetwork, 0, 0)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	e.handleMu.Lock()
	e.handle = h
	e.doneCh = done
	e.handleMu.Unlock()

	go e.runLoop(ctx, h, done)
	go e.runHousekeeping(ctx, h)
	return nil
}

func (e *networkEngine) Stop() error {
	e.handleMu.Lock()
	h := e.handle
	done := e.doneCh
	e.handle = 0
	e.doneCh = nil
	e.handleMu.Unlock()

	if h != 0 {
		_ = e.dll.Shutdown(h, winDivertShutdownBoth)
		_ = e.dll.Close(h)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

// OnSocketMapped is invoked by SocketTable when a new PID mapping arrives,
// draining any staged first packets (TCP SYN / QUIC Initial) immediately.
func (e *networkEngine) OnSocketMapped(key SocketKey, pid uint32) {
	staged := e.staging.DrainForSocket(key)
	if len(staged) == 0 {
		return
	}
	e.handleMu.Lock()
	h := e.handle
	e.handleMu.Unlock()
	if h == 0 {
		return
	}

	for _, sp := range staged {
		var addr winDivertAddress
		addr.SetOutbound(sp.Outbound)
		addr.Network().IfIdx = sp.IfIdx
		addr.Network().SubIfIdx = sp.SubIfIdx
		if parsed, ok := ParseIPv4Transport(sp.RawPacket); ok {
			e.processOutboundWithPID(h, sp.RawPacket, parsed, &addr, pid)
		}
	}
}

func (e *networkEngine) runLoop(ctx context.Context, handle uintptr, done chan struct{}) {
	defer close(done)
	buf := make([]byte, networkRecvBufSize)
	var addr winDivertAddress

	for {
		if ctx.Err() != nil {
			return
		}
		n, err := e.dll.Recv(handle, buf, &addr)
		if err != nil {
			return
		}
		pkt := buf[:n]

		parsed, ok := ParseIPv4Transport(pkt)
		if !ok {
			// Pass through non-TCP/UDP or malformed frames untouched
			_, _ = e.dll.Send(handle, pkt, &addr)
			continue
		}

		e.stateMu.RLock()
		policy := e.policy
		topo := e.topology
		dnsHook := e.dnsHook
		e.stateMu.RUnlock()

		// Optional DNS Guard hook (T11)
		if dnsHook != nil && dnsHook(pkt, parsed, &addr, topo, policy) {
			continue
		}

		if addr.Outbound() {
			e.handleOutbound(handle, pkt, parsed, &addr)
		} else {
			e.handleInbound(handle, pkt, parsed, &addr)
		}
	}
}

func (e *networkEngine) handleOutbound(handle uintptr, pkt []byte, parsed ParsedIPv4Packet, addr *winDivertAddress) {
	e.stateMu.RLock()
	policy := e.policy
	topo := e.topology
	tunnelUp := e.tunnelUp
	e.stateMu.RUnlock()

	if policy == nil || !policy.Enabled() || policy.Mode() == "global" {
		e.reinjectOriginal(handle, pkt, addr, false)
		return
	}

	// Fast path: check existing NAT session first (zero PID lookup overhead for established flows!)
	origFlow := FlowKey{
		Proto:      parsed.Proto,
		LocalAddr:  parsed.SrcIP,
		LocalPort:  parsed.SrcPort,
		RemoteAddr: parsed.DstIP,
		RemotePort: parsed.DstPort,
	}
	if entry, ok := e.nat.LookupOutbound(origFlow); ok {
		if entry.Decision.UseVPN() {
			if !tunnelUp || topo == nil || topo.Tunnel == nil {
				if policy.KillSwitchFailClosed() {
					e.packetsDropped.Add(1)
					return
				}
				e.reinjectOriginal(handle, pkt, addr, false)
				return
			}
			e.injectNATOutbound(handle, pkt, parsed, addr, entry)
			return
		}
		// Exclude mode direct NAT session (rerouted to physical NIC)
		if entry.TargetIfIdx != 0 && entry.NATLocalAddr.IsValid() {
			e.injectNATOutbound(handle, pkt, parsed, addr, entry)
			return
		}
		e.reinjectOriginal(handle, pkt, addr, false)
		return
	}

	// Slow path (new flow): resolve owning PID via SocketTracker + iphlpapi fallback
	sk := SocketKey{Proto: parsed.Proto, LocalPort: parsed.SrcPort}
	pid, ok := e.tracker.LookupWithFallback(sk)
	if !ok {
		// Hold briefly in StagingQueue (25ms) waiting for WINDIVERT_LAYER_SOCKET event
		if e.staging.Enqueue(StagedPacket{
			Key:       sk,
			RawPacket: pkt,
			IfIdx:     addr.Network().IfIdx,
			SubIfIdx:  addr.Network().SubIfIdx,
			Outbound:  true,
		}) {
			e.stagingQueued.Add(1)
			return
		}
		// Queue saturated: fail-closed drops packet so TCP/QUIC safely retries
		if policy.KillSwitchFailClosed() {
			e.packetsDropped.Add(1)
			return
		}
		e.reinjectOriginal(handle, pkt, addr, false)
		return
	}

	e.processOutboundWithPID(handle, pkt, parsed, addr, pid)
}

func (e *networkEngine) processOutboundWithPID(handle uintptr, pkt []byte, parsed ParsedIPv4Packet, addr *winDivertAddress, pid uint32) {
	e.stateMu.RLock()
	policy := e.policy
	topo := e.topology
	tunnelUp := e.tunnelUp
	e.stateMu.RUnlock()

	if policy == nil {
		e.reinjectOriginal(handle, pkt, addr, false)
		return
	}

	procInfo, err := e.resolver.Resolve(pid)
	if err != nil {
		// Unresolvable system/PPL process -> pass through in include mode, or fail-closed in exclude mode
		if policy.Mode() == "include" {
			e.reinjectOriginal(handle, pkt, addr, false)
			return
		}
		if policy.KillSwitchFailClosed() && !tunnelUp {
			e.packetsDropped.Add(1)
			return
		}
		e.reinjectOriginal(handle, pkt, addr, false)
		return
	}

	decision := policy.Evaluate(procInfo)
	if decision == appmgr.DecisionBypassAntiLoop {
		e.reinjectOriginal(handle, pkt, addr, false)
		return
	}

	origFlow := FlowKey{
		Proto:      parsed.Proto,
		LocalAddr:  parsed.SrcIP,
		LocalPort:  parsed.SrcPort,
		RemoteAddr: parsed.DstIP,
		RemotePort: parsed.DstPort,
	}

	// Case A: Include mode — selected app MUST go to Wintun (VPN), unselected stays direct natively
	if policy.Mode() == "include" {
		if !decision.UseVPN() {
			e.reinjectOriginal(handle, pkt, addr, false)
			return
		}
		if !tunnelUp || topo == nil || topo.Tunnel == nil || !topo.Tunnel.IPv4.IsValid() {
			if policy.KillSwitchFailClosed() {
				e.packetsDropped.Add(1)
				return
			}
			e.reinjectOriginal(handle, pkt, addr, false)
			return
		}
		entry, _ := e.nat.GetOrCreateOutbound(
			origFlow,
			topo.Tunnel.IPv4,
			topo.Tunnel.IfIndex,
			addr.Network().IfIdx,
			pid,
			decision,
		)
		e.injectNATOutbound(handle, pkt, parsed, addr, entry)
		return
	}

	// Case B: Exclude mode — default route is Wintun; excluded app MUST be steered to Physical NIC
	if policy.Mode() == "exclude" {
		if decision.UseVPN() {
			if !tunnelUp && policy.KillSwitchFailClosed() {
				e.packetsDropped.Add(1)
				return
			}
			e.reinjectOriginal(handle, pkt, addr, true)
			return
		}
		// Excluded app -> SNAT to Physical NIC
		if topo != nil && topo.Physical != nil && topo.Physical.IPv4.IsValid() {
			entry, _ := e.nat.GetOrCreateOutbound(
				origFlow,
				topo.Physical.IPv4,
				topo.Physical.IfIndex,
				addr.Network().IfIdx,
				pid,
				decision,
			)
			e.injectNATOutbound(handle, pkt, parsed, addr, entry)
			return
		}
	}

	e.reinjectOriginal(handle, pkt, addr, decision.UseVPN())
}

func (e *networkEngine) handleInbound(handle uintptr, pkt []byte, parsed ParsedIPv4Packet, addr *winDivertAddress) {
	// For inbound reply packets, Local is Dst and Remote is Src
	inFlow := FlowKey{
		Proto:      parsed.Proto,
		LocalAddr:  parsed.DstIP,
		LocalPort:  parsed.DstPort,
		RemoteAddr: parsed.SrcIP,
		RemotePort: parsed.SrcPort,
	}
	entry, ok := e.nat.LookupInbound(inFlow)
	if !ok {
		_, _ = e.dll.Send(handle, pkt, addr)
		return
	}

	if !RewriteIPv4DNAT(pkt, parsed.IHL, parsed.Proto, entry.OrigLocalAddr, entry.OrigLocalPort) {
		e.packetsDropped.Add(1)
		return
	}
	addr.SetImpostor(true)
	addr.Network().IfIdx = entry.ReturnIfIdx
	addr.Network().SubIfIdx = 0
	if _, err := e.dll.Send(handle, pkt, addr); err != nil {
		e.packetsDropped.Add(1)
	}
}

func (e *networkEngine) injectNATOutbound(handle uintptr, pkt []byte, parsed ParsedIPv4Packet, addr *winDivertAddress, entry NATEntry) {
	if !RewriteIPv4SNAT(pkt, parsed.IHL, parsed.Proto, entry.NATLocalAddr, entry.NATLocalPort) {
		e.packetsDropped.Add(1)
		return
	}
	addr.SetImpostor(true)
	addr.SetOutbound(true)
	addr.Network().IfIdx = entry.TargetIfIdx
	addr.Network().SubIfIdx = 0

	if _, err := e.dll.Send(handle, pkt, addr); err != nil {
		// Fail-closed: NEVER fall back to original interface if target injection fails!
		e.packetsDropped.Add(1)
		return
	}
	if entry.Decision.UseVPN() {
		e.packetsToVPN.Add(1)
	} else {
		e.packetsDirect.Add(1)
	}
}

func (e *networkEngine) reinjectOriginal(handle uintptr, pkt []byte, addr *winDivertAddress, viaVPN bool) {
	if _, err := e.dll.Send(handle, pkt, addr); err != nil {
		e.packetsDropped.Add(1)
		return
	}
	if viaVPN {
		e.packetsToVPN.Add(1)
	} else {
		e.packetsDirect.Add(1)
	}
}

func (e *networkEngine) runHousekeeping(ctx context.Context, handle uintptr) {
	stagingTicker := time.NewTicker(stagingSweepInterval)
	natTicker := time.NewTicker(natSweepInterval)
	defer stagingTicker.Stop()
	defer natTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-stagingTicker.C:
			expired := e.staging.PopExpired(now)
			if len(expired) == 0 {
				continue
			}
			e.stateMu.RLock()
			policy := e.policy
			e.stateMu.RUnlock()

			for _, sp := range expired {
				e.stagingExpired.Add(1)
				// Final fallback attempt before expiry drop
				if pid, ok := e.tracker.LookupWithFallback(sp.Key); ok {
					var addr winDivertAddress
					addr.SetOutbound(sp.Outbound)
					addr.Network().IfIdx = sp.IfIdx
					addr.Network().SubIfIdx = sp.SubIfIdx
					if parsed, ok2 := ParseIPv4Transport(sp.RawPacket); ok2 {
						e.processOutboundWithPID(handle, sp.RawPacket, parsed, &addr, pid)
						continue
					}
				}
				if policy != nil && !policy.KillSwitchFailClosed() {
					var addr winDivertAddress
					addr.SetOutbound(sp.Outbound)
					addr.Network().IfIdx = sp.IfIdx
					addr.Network().SubIfIdx = sp.SubIfIdx
					_, _ = e.dll.Send(handle, sp.RawPacket, &addr)
				} else {
					// Fail-closed: drop unresolved first packet; TCP SYN / QUIC Initial retries cleanly
					e.packetsDropped.Add(1)
				}
			}
		case now := <-natTicker.C:
			e.nat.SweepExpired(now)
		}
	}
}

var _ = netip.Addr{}
