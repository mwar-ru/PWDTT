//go:build windows

package vpn

import "testing"

func TestWindowsInspectorFindsActivePhysicalNIC(t *testing.T) {
	ins := NewWindowsInspector("PWDTT")
	topo, err := ins.Inspect()
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if len(topo.All) == 0 {
		t.Fatalf("expected non-empty Windows interface list")
	}
	if topo.Physical == nil {
		t.Fatalf("expected an active physical NIC on Windows")
	}
	t.Logf("Detected Physical NIC: %s (IfIdx=%d, IPv4=%s, GW=%s, Metric=%d, CompetingVPNs=%d)",
		topo.Physical.Name, topo.Physical.IfIndex, topo.Physical.IPv4, topo.Physical.GatewayIPv4,
		topo.Physical.Metric, len(topo.CompetingVPNs))
}
