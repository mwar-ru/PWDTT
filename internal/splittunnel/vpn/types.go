package vpn

import (
	"net/netip"
	"sort"
	"strings"
)

// TunnelState represents the lifecycle state of the PWDTT WireGuard/Wintun tunnel.
type TunnelState int

const (
	StateDown TunnelState = iota
	StateStarting
	StateUp
	StateReconnecting
	StateError
)

// String returns a human-readable tunnel state name.
func (s TunnelState) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateUp:
		return "up"
	case StateReconnecting:
		return "reconnecting"
	case StateError:
		return "error"
	default:
		return "down"
	}
}

// InterfaceInfo describes a single Windows network adapter relevant to split tunneling.
type InterfaceInfo struct {
	IfIndex        uint32       `json:"if_index"`
	LUID           uint64       `json:"luid"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	IPv4           netip.Addr   `json:"ipv4,omitempty"`
	IPv6           netip.Addr   `json:"ipv6,omitempty"`
	GatewayIPv4    netip.Addr   `json:"gateway_ipv4,omitempty"`
	GatewayIPv6    netip.Addr   `json:"gateway_ipv6,omitempty"`
	DNSServers     []netip.Addr `json:"-"`
	Metric         uint32       `json:"metric"`
	MTU            uint32       `json:"mtu"`
	IsUp           bool         `json:"is_up"`
	IsLoopback     bool         `json:"is_loopback"`
	IsVirtual      bool         `json:"is_virtual"`
	IsCompetingVPN bool         `json:"is_competing_vpn"`
}

// NetworkTopology captures the active PWDTT Wintun adapter, best physical NIC, and competing VPN warnings.
type NetworkTopology struct {
	Tunnel        *InterfaceInfo  `json:"tunnel,omitempty"`
	Physical      *InterfaceInfo  `json:"physical,omitempty"`
	CompetingVPNs []InterfaceInfo `json:"competing_vpns,omitempty"`
	All           []InterfaceInfo `json:"all,omitempty"`
}

// HasIPv6Tunnel reports whether the active Wintun tunnel has a valid IPv6 address.
func (t NetworkTopology) HasIPv6Tunnel() bool {
	return t.Tunnel != nil && t.Tunnel.IPv6.IsValid()
}

// TunnelEvent is emitted by the PWDTT upstream adapter (T06) when tunnel state changes.
type TunnelEvent struct {
	State    TunnelState
	Topology *NetworkTopology
	Err      error
}

// InterfaceProvider abstracts OS network interface enumeration for deterministic unit testing.
type InterfaceProvider interface {
	ListInterfaces() ([]InterfaceInfo, error)
}

// Inspector resolves active Wintun and physical Windows network adapters.
type Inspector struct {
	provider       InterfaceProvider
	tunnelNameHint string
}

// NewInspector creates an Inspector backed by the provided InterfaceProvider.
func NewInspector(provider InterfaceProvider, tunnelNameHint string) *Inspector {
	return &Inspector{
		provider:       provider,
		tunnelNameHint: strings.TrimSpace(tunnelNameHint),
	}
}

// Inspect enumerates interfaces and computes the current NetworkTopology.
func (ins *Inspector) Inspect() (NetworkTopology, error) {
	ifaces, err := ins.provider.ListInterfaces()
	if err != nil {
		return NetworkTopology{}, err
	}
	return SelectTopology(ifaces, ins.tunnelNameHint), nil
}

// ClassifyVirtualAdapter determines whether an adapter is virtual (hypervisor/WSL/Docker/VPN)
// and whether it looks like a competing VPN TUN/TAP adapter.
func ClassifyVirtualAdapter(name, description string) (isVirtual bool, isCompetingVPN bool) {
	combined := strings.ToLower(name + " " + description)

	// Virtual VPN/TUN/TAP signatures
	vpnKeywords := []string{
		"wintun",
		"wireguard",
		"tailscale",
		"openvpn",
		"tap-windows",
		"amnezia",
		"nordlynx",
		"mullvad",
		"cloudflare warp",
		"warp",
		"outline",
		"sing-box",
		"clash",
		"nekoray",
		"v2ray",
		"zerotier",
		"hamachi",
		"radmin vpn",
		"anyconnect",
		"fortinet",
		"globalprotect",
		"protonvpn",
	}
	for _, kw := range vpnKeywords {
		if strings.Contains(combined, kw) {
			return true, true
		}
	}

	// Non-VPN local virtual switches (Docker, WSL, Hyper-V, VMware, VirtualBox)
	localVirtualKeywords := []string{
		"vethernet",
		"hyper-v",
		"wsl",
		"vmware",
		"virtualbox",
		"vbox",
		"docker",
		"loopback",
		"teredo",
		"isatap",
		"6to4",
		"bluetooth",
		"microsoft wi-fi direct",
		"npcap",
		"km-test",
	}
	for _, kw := range localVirtualKeywords {
		if strings.Contains(combined, kw) {
			return true, false
		}
	}

	return false, false
}

// SelectTopology picks the PWDTT Wintun tunnel, the best active physical NIC, and any competing VPNs.
func SelectTopology(ifaces []InterfaceInfo, tunnelNameHint string) NetworkTopology {
	topo := NetworkTopology{
		All: make([]InterfaceInfo, len(ifaces)),
	}
	copy(topo.All, ifaces)

	hintLower := strings.ToLower(strings.TrimSpace(tunnelNameHint))

	// 1. Identify PWDTT Wintun tunnel adapter
	tunnelIdx := -1
	bestTunnelScore := 0
	for i := range topo.All {
		iface := &topo.All[i]
		if !iface.IsUp || iface.IsLoopback {
			continue
		}
		nameLower := strings.ToLower(iface.Name)
		descLower := strings.ToLower(iface.Description)

		score := 0
		if hintLower != "" && nameLower == hintLower {
			score = 100
		} else if hintLower != "" && (strings.Contains(nameLower, hintLower) || strings.Contains(descLower, hintLower)) {
			score = 80
		} else if strings.Contains(nameLower, "pwdtt") || strings.Contains(descLower, "pwdtt") {
			score = 70
		} else if strings.Contains(descLower, "wintun") && hintLower == "" {
			score = 50
		}

		if score > bestTunnelScore {
			bestTunnelScore = score
			tunnelIdx = i
		}
	}

	if tunnelIdx >= 0 {
		cp := cloneInterfaceInfo(topo.All[tunnelIdx])
		topo.Tunnel = &cp
	}

	// 2. Collect competing active VPN adapters (excluding our own tunnel)
	for i := range topo.All {
		if i == tunnelIdx {
			continue
		}
		iface := topo.All[i]
		if !iface.IsUp || iface.IsLoopback {
			continue
		}
		isVirt, isVPN := ClassifyVirtualAdapter(iface.Name, iface.Description)
		if iface.IsCompetingVPN || (isVirt && isVPN) {
			topo.CompetingVPNs = append(topo.CompetingVPNs, cloneInterfaceInfo(iface))
		}
	}

	// 3. Select best active physical NIC (lowest metric with IPv4 + default gateway preferred)
	var physicalCandidates []InterfaceInfo
	for i := range topo.All {
		if i == tunnelIdx {
			continue
		}
		iface := topo.All[i]
		if !iface.IsUp || iface.IsLoopback {
			continue
		}
		isVirt, _ := ClassifyVirtualAdapter(iface.Name, iface.Description)
		if iface.IsVirtual || isVirt {
			continue
		}
		if !iface.IPv4.IsValid() && !iface.IPv6.IsValid() {
			continue
		}
		physicalCandidates = append(physicalCandidates, cloneInterfaceInfo(iface))
	}

	sort.Slice(physicalCandidates, func(i, j int) bool {
		a, b := physicalCandidates[i], physicalCandidates[j]
		aHasGW := a.GatewayIPv4.IsValid()
		bHasGW := b.GatewayIPv4.IsValid()
		if aHasGW != bHasGW {
			return aHasGW
		}
		aHasV4 := a.IPv4.IsValid()
		bHasV4 := b.IPv4.IsValid()
		if aHasV4 != bHasV4 {
			return aHasV4
		}
		if a.Metric != b.Metric {
			return a.Metric < b.Metric
		}
		return a.IfIndex < b.IfIndex
	})

	if len(physicalCandidates) > 0 {
		best := physicalCandidates[0]
		topo.Physical = &best
	}

	return topo
}

func cloneInterfaceInfo(in InterfaceInfo) InterfaceInfo {
	cp := in
	if len(in.DNSServers) > 0 {
		cp.DNSServers = append([]netip.Addr(nil), in.DNSServers...)
	}
	return cp
}
