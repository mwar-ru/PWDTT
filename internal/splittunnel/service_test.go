package splittunnel

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"pwdtt/internal/splittunnel/router"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

type fakeTrafficRouter struct {
	mu           sync.Mutex
	started      bool
	startCalls   int
	stopCalls    int
	appliedCfgs  []settings.Config
	tunnelEvents []vpn.TunnelEvent
}

func (f *fakeTrafficRouter) Start(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
	f.startCalls++
	return nil
}

func (f *fakeTrafficRouter) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = false
	f.stopCalls++
	return nil
}

func (f *fakeTrafficRouter) ApplyConfig(cfg settings.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appliedCfgs = append(f.appliedCfgs, cfg)
	return nil
}

func (f *fakeTrafficRouter) OnTunnelEvent(ev vpn.TunnelEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tunnelEvents = append(f.tunnelEvents, ev)
}

func (f *fakeTrafficRouter) Stats() router.RouterStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := router.RouterStopped
	if f.started {
		state = router.RouterRunning
	}
	return router.RouterStats{State: state}
}

func TestServiceLifecycleAndTunnelEvents(t *testing.T) {
	baseDir := t.TempDir()
	store, err := settings.NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fakeTR := &fakeTrafficRouter{}
	adapter := vpn.NewPWDTTAdapter(vpn.NewWindowsInspector(vpn.DefaultTunnelInterfaceName))

	svc := NewServiceWithDeps(store, nil, adapter, fakeTR, true)
	svc.Startup(context.Background())
	defer svc.Shutdown(context.Background())

	if !svc.IsSupported() {
		t.Fatal("expected IsSupported() == true")
	}
	if !vpn.ShouldSkipDefaultRoute() {
		t.Fatal("expected default enabled include mode to skip WireGuard default route")
	}

	fakeTR.mu.Lock()
	if !fakeTR.started || fakeTR.startCalls != 1 {
		t.Fatalf("expected router started once in default include mode, got started=%v calls=%d", fakeTR.started, fakeTR.startCalls)
	}
	fakeTR.mu.Unlock()

	adapter.OnTunnelState(vpn.StateStarting, vpn.DefaultTunnelInterfaceName, nil)
	adapter.OnTunnelState(vpn.StateUp, vpn.DefaultTunnelInterfaceName, nil)
	adapter.OnTunnelState(vpn.StateDown, vpn.DefaultTunnelInterfaceName, nil)

	fakeTR.mu.Lock()
	evCount := len(fakeTR.tunnelEvents)
	fakeTR.mu.Unlock()
	if evCount != 3 {
		t.Fatalf("expected 3 tunnel events forwarded to router, got %d", evCount)
	}

	st := svc.GetStatus()
	if !st.Running || st.TunnelState != vpn.StateDown {
		t.Fatalf("unexpected status snapshot: %+v", st)
	}

	svc.Shutdown(context.Background())
	if vpn.ShouldSkipDefaultRoute() {
		t.Fatal("expected Shutdown to clear active tunnel hooks")
	}
}

func TestServiceSaveConfigDynamicStartStop(t *testing.T) {
	store, err := settings.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	fakeTR := &fakeTrafficRouter{}
	adapter := vpn.NewPWDTTAdapter(vpn.NewWindowsInspector(vpn.DefaultTunnelInterfaceName))
	svc := NewServiceWithDeps(store, nil, adapter, fakeTR, true)
	svc.Startup(context.Background())
	defer svc.Shutdown(context.Background())

	// Switch to global -> router stops and default route is no longer skipped
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	cfg.Mode = "global"
	saved, err := svc.SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig(global): %v", err)
	}
	if saved.Mode != "global" {
		t.Fatalf("expected mode=global, got %s", saved.Mode)
	}
	if svc.GetStatus().Running {
		t.Fatal("expected router to stop in global mode")
	}
	if vpn.ShouldSkipDefaultRoute() {
		t.Fatal("global mode must not skip default route")
	}

	// Switch back to include -> router starts and skips default route
	cfg.Mode = "include"
	cfg.Enabled = true
	cfg.Apps = []settings.AppConfig{{ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`, MatchChildren: true}}
	if _, err := svc.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig(include): %v", err)
	}
	if !svc.GetStatus().Running {
		t.Fatal("expected router to run in enabled include mode")
	}
	if !vpn.ShouldSkipDefaultRoute() {
		t.Fatal("include mode must skip default route")
	}
}

func TestServiceSetConfigPathPreservesOneDriveConfig(t *testing.T) {
	store, err := settings.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	oneDriveDir := t.TempDir()
	syncedPath := filepath.Join(oneDriveDir, "split_tunnel.json")
	syncedJSON := `{"schema_version":1,"enabled":true,"mode":"exclude","kill_switch":"fail_closed","wfp_crash_mode":"dynamic","system_dns":"tunnel_dns","portable_env_paths":true,"apps":[{"exe_path":"C:\\Games\\game.exe","match_children":true}]}`
	if err := os.WriteFile(syncedPath, []byte(syncedJSON), 0o600); err != nil {
		t.Fatalf("write synced config: %v", err)
	}

	fakeTR := &fakeTrafficRouter{}
	svc := NewServiceWithDeps(store, nil, nil, fakeTR, true)
	svc.Startup(context.Background())
	defer svc.Shutdown(context.Background())

	loaded, err := svc.SetConfigPath(syncedPath, false)
	if err != nil {
		t.Fatalf("SetConfigPath failed: %v", err)
	}
	if loaded.Mode != "exclude" || len(loaded.Apps) != 1 {
		t.Fatalf("expected preserved OneDrive config (mode=exclude, 1 app), got %+v", loaded)
	}
	if svc.GetConfigPath() != syncedPath {
		t.Fatalf("expected active path %s, got %s", syncedPath, svc.GetConfigPath())
	}
}
