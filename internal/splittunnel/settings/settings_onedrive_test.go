package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetConfigPathPreservesExistingOneDriveConfig(t *testing.T) {
	pc1Base := t.TempDir()
	pc2Base := t.TempDir()
	oneDriveDir := t.TempDir()
	sharedPath := filepath.Join(oneDriveDir, "split_tunnel.json")

	// PC #1 saves Firefox + Edge to shared OneDrive config
	store1, err := NewStore(pc1Base)
	if err != nil {
		t.Fatalf("NewStore pc1: %v", err)
	}
	if err := store1.SetConfigPath(sharedPath, false); err != nil {
		t.Fatalf("SetConfigPath pc1: %v", err)
	}
	cfg1 := DefaultConfig()
	cfg1.Mode = "include"
	cfg1.Apps = []AppConfig{
		{Name: "Firefox", ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`, MatchChildren: true},
		{Name: "Edge", ExePath: `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, MatchChildren: true},
	}
	if err := store1.Save(&cfg1); err != nil {
		t.Fatalf("Save pc1: %v", err)
	}

	// PC #2 points to the existing OneDrive config with moveExisting=false
	store2, err := NewStore(pc2Base)
	if err != nil {
		t.Fatalf("NewStore pc2: %v", err)
	}
	if err := store2.SetConfigPath(sharedPath, false); err != nil {
		t.Fatalf("SetConfigPath pc2: %v", err)
	}

	got := store2.GetConfig()
	if len(got.Apps) != 2 || got.Apps[0].Name != "Firefox" || got.Apps[1].Name != "Edge" {
		t.Fatalf("expected PC #2 to preserve existing OneDrive apps, got: %+v", got.Apps)
	}
}

func TestLoadReturnsDefensiveCopyAndNormalizesInvalidEnums(t *testing.T) {
	baseDir := t.TempDir()
	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	raw := `{"schema_version":0,"enabled":true,"mode":"invalid_mode","kill_switch":"","wfp_crash_mode":"bad","system_dns":"","apps":[]}`
	if err := os.WriteFile(filepath.Join(baseDir, "split_tunnel.json"), []byte(raw), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Mode != "include" || loaded.KillSwitch != "fail_closed" || loaded.WFPCrashMode != "dynamic" || loaded.SystemDNS != "tunnel_dns" {
		t.Fatalf("unexpected normalized defaults: %+v", loaded)
	}

	// Mutating returned pointer must not corrupt internal store state
	loaded.Mode = "exclude"
	if store.GetConfig().Mode != "include" {
		t.Fatalf("Load() leaked internal Store pointer")
	}
}
