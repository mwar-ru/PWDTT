//go:build windows

package appmgr

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsProcessResolverSelfAndRunningApps(t *testing.T) {
	resolver := NewWindowsProcessResolver()
	selfPID := uint32(os.Getpid())

	info, err := resolver.Resolve(selfPID)
	if err != nil {
		t.Fatalf("Resolve(selfPID=%d) failed: %v", selfPID, err)
	}
	if info.ExePath == "" || !strings.HasSuffix(info.ExePath, ".exe") {
		t.Fatalf("unexpected Windows self ExePath: %q", info.ExePath)
	}

	apps, err := resolver.ListRunningApps()
	if err != nil {
		t.Fatalf("ListRunningApps failed: %v", err)
	}
	if len(apps) == 0 {
		t.Fatalf("expected at least 1 running app on Windows")
	}
}
