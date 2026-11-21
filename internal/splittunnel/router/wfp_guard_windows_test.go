//go:build windows

package router

import (
	"runtime"
	"testing"
	"unsafe"
)

// TestWFPStructABILayoutAMD64 verifies exact MSVC windows/amd64 C struct sizes and field offsets
// without opening a live WFP session or requiring Administrator privileges.
func TestWFPStructABILayoutAMD64(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("ABI size assertions target windows/amd64")
	}

	if got := unsafe.Sizeof(fwpByteBlob{}); got != 16 {
		t.Fatalf("sizeof(FWP_BYTE_BLOB) = %d, want 16", got)
	}
	if got := unsafe.Sizeof(fwpmDisplayData0{}); got != 16 {
		t.Fatalf("sizeof(FWPM_DISPLAY_DATA0) = %d, want 16", got)
	}
	if got := unsafe.Sizeof(fwpmSession0{}); got != 72 {
		t.Fatalf("sizeof(FWPM_SESSION0) = %d, want 72", got)
	}
	if got := unsafe.Sizeof(fwpmProvider0{}); got != 64 {
		t.Fatalf("sizeof(FWPM_PROVIDER0) = %d, want 64", got)
	}
	if got := unsafe.Sizeof(fwpmSublayer0{}); got != 72 {
		t.Fatalf("sizeof(FWPM_SUBLAYER0) = %d, want 72", got)
	}
	if got := unsafe.Sizeof(fwpValue0{}); got != 16 {
		t.Fatalf("sizeof(FWP_VALUE0) = %d, want 16", got)
	}
	if got := unsafe.Sizeof(fwpmFilterCondition0{}); got != 40 {
		t.Fatalf("sizeof(FWPM_FILTER_CONDITION0) = %d, want 40", got)
	}
	if got := unsafe.Sizeof(fwpmAction0{}); got != 20 {
		t.Fatalf("sizeof(FWPM_ACTION0) = %d, want 20", got)
	}

	var f fwpmFilter0
	if got := unsafe.Sizeof(f); got != 200 {
		t.Fatalf("sizeof(FWPM_FILTER0) = %d, want 200", got)
	}
	if got := unsafe.Offsetof(f.ProviderContextKey); got != 152 {
		t.Fatalf("offsetof(FWPM_FILTER0.ProviderContextKey) = %d, want 152", got)
	}
	if got := unsafe.Offsetof(f.FilterId); got != 176 {
		t.Fatalf("offsetof(FWPM_FILTER0.FilterId) = %d, want 176", got)
	}
	if got := unsafe.Offsetof(f.EffectiveWeight); got != 184 {
		t.Fatalf("offsetof(FWPM_FILTER0.EffectiveWeight) = %d, want 184", got)
	}
}
