package vpn

import (
	"errors"
	"net/netip"
	"sync"
	"testing"
)

func TestPWDTTAdapterModeControlsDefaultRouteSkip(t *testing.T) {
	adapter := NewPWDTTAdapter(NewInspector(&mockInterfaceProvider{}, DefaultTunnelInterfaceName))
	SetActiveHooks(adapter)
	defer SetActiveHooks(nil)

	// Default: off -> never skip default route (100% original PWDTT)
	if ShouldSkipDefaultRoute() {
		t.Fatalf("expected ShouldSkipDefaultRoute=false by default")
	}

	// Enabled + include -> MUST skip default route
	adapter.SetSplitTunnelMode(true, "include")
	if !ShouldSkipDefaultRoute() {
		t.Fatalf("expected ShouldSkipDefaultRoute=true in enabled include mode")
	}

	// Enabled + global / exclude -> MUST NOT skip default route
	adapter.SetSplitTunnelMode(true, "global")
	if ShouldSkipDefaultRoute() {
		t.Fatalf("expected ShouldSkipDefaultRoute=false in global mode")
	}
	adapter.SetSplitTunnelMode(true, "exclude")
	if ShouldSkipDefaultRoute() {
		t.Fatalf("expected ShouldSkipDefaultRoute=false in exclude mode")
	}

	// Disabled + include -> MUST NOT skip default route
	adapter.SetSplitTunnelMode(false, "include")
	if ShouldSkipDefaultRoute() {
		t.Fatalf("expected ShouldSkipDefaultRoute=false when split tunneling is disabled")
	}
}

func TestIsDefaultRouteCIDR(t *testing.T) {
	yes := []string{"0.0.0.0/0", "0.0.0.0/1", "128.0.0.0/1", "::/0", "::/1", "8000::/1"}
	for _, c := range yes {
		if !IsDefaultRouteCIDR(c) {
			t.Fatalf("expected %q to be treated as default route CIDR", c)
		}
	}
	no := []string{"10.0.0.0/8", "192.168.1.0/24", "1.1.1.1/32", "fd00::/8"}
	for _, c := range no {
		if IsDefaultRouteCIDR(c) {
			t.Fatalf("expected %q NOT to be treated as default route CIDR", c)
		}
	}
}

func TestPWDTTAdapterLifecycleEventsAndTopology(t *testing.T) {
	provider := &mockInterfaceProvider{
		ifaces: []InterfaceInfo{
			{
				IfIndex:     10,
				Name:        "Ethernet",
				Description: "Intel Ethernet",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("192.168.1.5"),
				GatewayIPv4: netip.MustParseAddr("192.168.1.1"),
				Metric:      10,
			},
			{
				IfIndex:     22,
				Name:        "pwdtt-wg",
				Description: "Wintun Userspace Tunnel",
				IsUp:        true,
				IPv4:        netip.MustParseAddr("10.66.66.2"),
				Metric:      9999,
			},
		},
	}

	adapter := NewPWDTTAdapter(NewInspector(provider, DefaultTunnelInterfaceName))
	adapter.inspectRetryWait = 0
	SetActiveHooks(adapter)
	defer SetActiveHooks(nil)

	var mu sync.Mutex
	var events []TunnelEvent
	unsub := adapter.Subscribe(func(ev TunnelEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
	defer unsub()

	NotifyTunnelState(StateStarting, DefaultTunnelInterfaceName, nil)
	NotifyTunnelState(StateUp, DefaultTunnelInterfaceName, nil)
	NotifyTunnelState(StateError, DefaultTunnelInterfaceName, errors.New("handshake timeout"))
	NotifyTunnelState(StateDown, DefaultTunnelInterfaceName, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 4 {
		t.Fatalf("expected 4 lifecycle events, got %d", len(events))
	}
	if events[0].State != StateStarting || events[1].State != StateUp || events[2].State != StateError || events[3].State != StateDown {
		t.Fatalf("unexpected event sequence: %+v", events)
	}
	if events[1].Topology == nil || events[1].Topology.Tunnel == nil || events[1].Topology.Tunnel.IfIndex != 22 {
		t.Fatalf("expected StateUp event to include pwdtt-wg topology, got %+v", events[1].Topology)
	}
	if events[2].Err == nil || events[2].Err.Error() != "handshake timeout" {
		t.Fatalf("expected error propagated in StateError event, got %v", events[2].Err)
	}
	if adapter.State() != StateDown || adapter.LastTopology() != nil {
		t.Fatalf("expected StateDown and cleared topology after teardown")
	}
}
