package vpn

import (
	"net/netip"
	"testing"
)

type mockInterfaceProvider struct {
	ifaces []InterfaceInfo
}

func (m *mockInterfaceProvider) ListInterfaces() ([]InterfaceInfo, error) {
	return m.ifaces, nil
}

func TestSelectTopologyBestPhysicalTunnelAndCompetingVPN(t *testing.T) {
	provider := &mockInterfaceProvider{
		ifaces: []InterfaceInfo{
			{
				IfIndex:     1,
				Name:        "Loopback Pseudo-Interface 1",
				IsUp:        true,
				IsLoopback:  true,
				IPv4:        netip.MustParseAddr("127.0.0.1"),
				Metric:      1,
			},
			{
				IfIndex:     5,
				Name:        "vEthernet (WSL)",
				Description: "Hyper-V Virtual Ethernet Adapter",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("172.24.0.1"),
				Metric:      5,
			},
			{
				IfIndex:     8,
				Name:        "Wi-Fi",
				Description: "Intel(R) Wi-Fi 6E AX211",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("192.168.1.42"),
				GatewayIPv4: netip.MustParseAddr("192.168.1.1"),
				Metric:      35,
			},
			{
				IfIndex:     12,
				Name:        "Ethernet",
				Description: "Realtek PCIe 2.5GbE Family Controller",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("192.168.1.10"),
				GatewayIPv4: netip.MustParseAddr("192.168.1.1"),
				Metric:      15,
			},
			{
				IfIndex:     14,
				Name:        "Ethernet 2",
				Description: "Disconnected NIC",
				IsUp:        false,
				IPv4:        netip.MustParseAddr("10.0.0.5"),
				GatewayIPv4: netip.MustParseAddr("10.0.0.1"),
				Metric:      5,
			},
			{
				IfIndex:     21,
				LUID:        0x0011223344556677,
				Name:        "PWDTT",
				Description: "Wintun Userspace Tunnel",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("10.66.66.2"),
				DNSServers:  []netip.Addr{netip.MustParseAddr("1.1.1.1")},
				Metric:      5,
				MTU:         1420,
			},
			{
				IfIndex:     25,
				Name:        "Tailscale",
				Description: "Wintun Userspace Tunnel",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("100.64.0.10"),
				Metric:      10,
			},
		},
	}

	ins := NewInspector(provider, "PWDTT")
	topo, err := ins.Inspect()
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}

	if topo.Tunnel == nil || topo.Tunnel.IfIndex != 21 || topo.Tunnel.Name != "PWDTT" {
		t.Fatalf("expected PWDTT tunnel (IfIndex=21), got %+v", topo.Tunnel)
	}
	if topo.HasIPv6Tunnel() {
		t.Fatalf("expected IPv4-only tunnel")
	}

	if topo.Physical == nil || topo.Physical.IfIndex != 12 || topo.Physical.Name != "Ethernet" {
		t.Fatalf("expected Ethernet (lowest metric 15) as Physical NIC, got %+v", topo.Physical)
	}

	if len(topo.CompetingVPNs) != 1 || topo.CompetingVPNs[0].Name != "Tailscale" {
		t.Fatalf("expected 1 competing VPN (Tailscale), got %+v", topo.CompetingVPNs)
	}
}

func TestClassifyVirtualAdapterDistinguishesWSLFromVPN(t *testing.T) {
	isVirt, isVPN := ClassifyVirtualAdapter("vEthernet (DockerNAT)", "Hyper-V Virtual Ethernet Adapter")
	if !isVirt || isVPN {
		t.Fatalf("DockerNAT expected virtual=true competingVPN=false, got virtual=%v vpn=%v", isVirt, isVPN)
	}

	isVirt2, isVPN2 := ClassifyVirtualAdapter("AmneziaWG", "Wintun Userspace Tunnel")
	if !isVirt2 || !isVPN2 {
		t.Fatalf("AmneziaWG expected virtual=true competingVPN=true, got virtual=%v vpn=%v", isVirt2, isVPN2)
	}

	isVirt3, isVPN3 := ClassifyVirtualAdapter("Ethernet", "Intel(R) Ethernet Controller I225-V")
	if isVirt3 || isVPN3 {
		t.Fatalf("Intel Ethernet expected virtual=false competingVPN=false, got virtual=%v vpn=%v", isVirt3, isVPN3)
	}
}
