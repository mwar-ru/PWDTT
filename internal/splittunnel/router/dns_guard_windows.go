//go:build windows

package router

import (
	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/vpn"
)

// newDNSPacketHook connects DNSGuard to the WinDivert NETWORK packet loop.
func newDNSPacketHook(guard *DNSGuard, eng *networkEngine) PacketHook {
	return func(
		pkt []byte,
		parsed ParsedIPv4Packet,
		addr *winDivertAddress,
		topo *vpn.NetworkTopology,
		policy *appmgr.PolicyEngine,
	) bool {
		if guard == nil || eng == nil || parsed.Proto != ProtoUDP {
			return false
		}

		// Inbound UDP/53 reply (fallback if it arrives via WinDivert)
		if !addr.Outbound() {
			if parsed.SrcPort != dnsPort {
				return false
			}
			returnIfIdx, ok := guard.RewriteInbound(pkt, parsed)
			if !ok {
				return false
			}
			addr.SetImpostor(true)
			addr.Network().IfIdx = returnIfIdx
			addr.Network().SubIfIdx = 0

			eng.handleMu.Lock()
			h := eng.handle
			eng.handleMu.Unlock()
			if h != 0 {
				if _, err := eng.dll.Send(h, pkt, addr); err != nil {
					eng.packetsDropped.Add(1)
				}
			}
			return true
		}

		// Outbound fast reject: skip everything except UDP/53 (DoH 443 untouched)
		if parsed.DstPort != dnsPort {
			return false
		}

		eng.stateMu.RLock()
		tunnelUp := eng.tunnelUp
		eng.stateMu.RUnlock()

		if !tunnelUp || policy == nil || !policy.Enabled() || policy.Mode() == "global" || policy.SystemDNSPolicy() != "tunnel_dns" {
			return false
		}

		pid, ok := eng.tracker.LookupWithFallback(SocketKey{
			Proto:     ProtoUDP,
			LocalPort: parsed.SrcPort,
		})
		if !ok {
			return false
		}

		procInfo, err := eng.resolver.Resolve(pid)
		if err != nil {
			return false
		}

		if !guard.ShouldInterceptOutbound(parsed, procInfo, policy, tunnelUp) {
			return false
		}

		if _, ok := guard.RewriteOutbound(pkt, parsed, addr.Network().IfIdx, topo); !ok {
			return false
		}

		// Feed redirected DNS query straight into wireguard-go tun.Device
		if !vpn.InjectOutboundPacket(pkt) {
			eng.packetsDropped.Add(1)
			return true
		}
		eng.dnsRedirected.Add(1)
		return true
	}
}
