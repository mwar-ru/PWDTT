//go:build windows

package router

import (
	"testing"
	"unsafe"
)

func TestWinDivert22ABILayoutAndBitfields(t *testing.T) {
	if got := unsafe.Sizeof(winDivertAddress{}); got != 80 {
		t.Fatalf("sizeof(WINDIVERT_ADDRESS) = %d, want 80", got)
	}
	if got := unsafe.Sizeof(winDivertDataNetwork{}); got != 8 {
		t.Fatalf("sizeof(WINDIVERT_DATA_NETWORK) = %d, want 8", got)
	}
	if got := unsafe.Sizeof(winDivertDataSocket{}); got != 64 {
		t.Fatalf("sizeof(WINDIVERT_DATA_SOCKET) = %d, want 64", got)
	}
	if got := unsafe.Sizeof(mibTCPRowOwnerPID{}); got != 24 {
		t.Fatalf("sizeof(MIB_TCPROW_OWNER_PID) = %d, want 24", got)
	}
	if got := unsafe.Sizeof(mibUDPRowOwnerPID{}); got != 12 {
		t.Fatalf("sizeof(MIB_UDPROW_OWNER_PID) = %d, want 12", got)
	}

	var addr winDivertAddress
	addr.Flags = uint32(winDivertLayerSocket) | (uint32(winDivertEventSocketConnect) << 8)
	addr.SetOutbound(true)
	addr.SetImpostor(true)

	if addr.Layer() != uint8(winDivertLayerSocket) || addr.Event() != winDivertEventSocketConnect {
		t.Fatalf("bitfield Layer/Event mismatch: layer=%d event=%d", addr.Layer(), addr.Event())
	}
	if !addr.Outbound() || !addr.Impostor() || addr.IPv6() {
		t.Fatalf("bitfield flags mismatch: outbound=%v impostor=%v ipv6=%v", addr.Outbound(), addr.Impostor(), addr.IPv6())
	}
}

func TestQueryWindowsSocketTableLive(t *testing.T) {
	tbl, err := QueryWindowsSocketTable()
	if err != nil {
		t.Fatalf("QueryWindowsSocketTable failed: %v", err)
	}
	if len(tbl) == 0 {
		t.Fatalf("expected active TCP/UDP sockets on Windows 11")
	}
	t.Logf("Enumerated %d active Windows TCP/UDP socket->PID entries via iphlpapi", len(tbl))
}
