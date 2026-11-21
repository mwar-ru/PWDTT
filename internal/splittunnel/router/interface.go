package router

import (
	"context"
	"net/netip"

	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

// Protocol identifies IP transport protocols handled by the split-tunneling router.
type Protocol uint8

const (
	ProtoTCP Protocol = 6
	ProtoUDP Protocol = 17
)

// String returns the protocol name.
func (p Protocol) String() string {
	switch p {
	case ProtoTCP:
		return "tcp"
	case ProtoUDP:
		return "udp"
	default:
		return "other"
	}
}

// RouterState represents the operational state of TrafficRouter.
type RouterState string

const (
	RouterStopped  RouterState = "stopped"
	RouterRunning  RouterState = "running"
	RouterDegraded RouterState = "degraded"
	RouterError    RouterState = "error"
)

// RouterStats holds a thread-safe snapshot of traffic routing telemetry for UI/diagnostics.
type RouterStats struct {
	State                    RouterState `json:"state"`
	Mode                     string      `json:"mode"`
	TunnelUp                 bool        `json:"tunnel_up"`
	PacketsToVPN             uint64      `json:"packets_to_vpn"`
	PacketsDirect            uint64      `json:"packets_direct"`
	PacketsDroppedFailClosed uint64      `json:"packets_dropped_fail_closed"`
	DNSRedirected            uint64      `json:"dns_redirected"`
	StagingQueued            uint64      `json:"staging_queued"`
	StagingExpired           uint64      `json:"staging_expired"`
	ActiveNATSessions        int         `json:"active_nat_sessions"`
	ActiveWFPFilters         int         `json:"active_wfp_filters"`
	LastError                string      `json:"last_error,omitempty"`
}

// FlowKey uniquely identifies an IPv4/IPv6 TCP or UDP 5-tuple.
type FlowKey struct {
	Proto      Protocol
	LocalAddr  netip.Addr
	LocalPort  uint16
	RemoteAddr netip.Addr
	RemotePort uint16
}

// SocketKey maps a local transport endpoint (from WINDIVERT_LAYER_SOCKET or IP Helper tables) to a PID.
type SocketKey struct {
	Proto     Protocol
	LocalPort uint16
}

// TrafficRouter is the core abstraction decoupling SplitTunnelService from WinDivert/WFP internals.
// A future WHQL-signed WFP Callout driver can implement this exact interface without touching other modules.
type TrafficRouter interface {
	Start(ctx context.Context) error
	Stop() error
	ApplyConfig(cfg settings.Config) error
	OnTunnelEvent(ev vpn.TunnelEvent)
	Stats() RouterStats
}
