package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func mockGetenv(mockVars map[string]string) func(string) string {
	return func(key string) string {
		if val, ok := mockVars[key]; ok {
			return val
		}
		return ""
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion 1, got %d", cfg.SchemaVersion)
	}
	if !cfg.Enabled {
		t.Errorf("expected Enabled true, got false")
	}
	if cfg.Mode != "include" {
		t.Errorf("expected Mode 'include', got '%s'", cfg.Mode)
	}
	if cfg.KillSwitch != "fail_closed" {
		t.Errorf("expected KillSwitch 'fail_closed', got '%s'", cfg.KillSwitch)
	}
	if cfg.WFPCrashMode != "dynamic" {
		t.Errorf("expected WFPCrashMode 'dynamic', got '%s'", cfg.WFPCrashMode)
	}
	if cfg.SystemDNS != "tunnel_dns" {
		t.Errorf("expected SystemDNS 'tunnel_dns', got '%s'", cfg.SystemDNS)
	}
	if !cfg.PortableEnvPaths {
		t.Errorf("expected PortableEnvPaths true, got false")
	}
	if cfg.Apps == nil {
		t.Errorf("expected non-nil Apps slice")
	}
}

func TestFoldAndExpandPath(t *testing.T) {
	env := map[string]string{
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"ProgramFiles":      `C:\Program Files`,
		"LocalAppData":     `C:\Users\TestUser\AppData\Local`,
		"AppData":          `C:\Users\TestUser\AppData\Roaming`,
		"UserProfile":      `C:\Users\TestUser`,
		"SystemRoot":       `C:\Windows`,
		"WinDir":           `C:\Windows`,
	}

	getenv := mockGetenv(env)

	tests := []struct {
		name         string
		input        string
		expectedFold string
		expectedExp  string
	}{
		{
			name:         "ProgramFiles(x86) path",
			input:        `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			expectedFold: `%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe`,
			expectedExp:  `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		},
		{
			name:         "ProgramFiles path",
			input:        `C:\Program Files\Mozilla Firefox\firefox.exe`,
			expectedFold: `%ProgramFiles%\Mozilla Firefox\firefox.exe`,
			expectedExp:  `C:\Program Files\Mozilla Firefox\firefox.exe`,
		},
		{
			name:         "Boundary check (Program Files Extra)",
			input:        `C:\Program Files Extra\app.exe`,
			expectedFold: `C:\Program Files Extra\app.exe`,
			expectedExp:  `C:\Program Files Extra\app.exe`,
		},
		{
			name:         "Exact match ProgramFiles",
			input:        `C:\Program Files`,
			expectedFold: `%ProgramFiles%`,
			expectedExp:  `C:\Program Files`,
		},
		{
			name:         "LocalAppData path",
			input:        `C:\Users\TestUser\AppData\Local\Programs\CustomApp\app.exe`,
			expectedFold: `%LocalAppData%\Programs\CustomApp\app.exe`,
			expectedExp:  `C:\Users\TestUser\AppData\Local\Programs\CustomApp\app.exe`,
		},
		{
			name:         "Unknown env variable",
			input:        `%UNKNOWN_VAR%\app.exe`,
			expectedFold: `%UNKNOWN_VAR%\app.exe`,
			expectedExp:  `%UNKNOWN_VAR%\app.exe`,
		},
		{
			name:         "Empty string",
			input:        "",
			expectedFold: "",
			expectedExp:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folded := foldPathWithEnv(tt.input, getenv)
			if folded != tt.expectedFold {
				t.Errorf("foldPathWithEnv(%q): expected %q, got %q", tt.input, tt.expectedFold, folded)
			}

			expanded := expandPathWithEnv(folded, getenv)
			if expanded != tt.expectedExp {
				t.Errorf("expandPathWithEnv(%q): expected %q, got %q", folded, tt.expectedExp, expanded)
			}
		})
	}
}

func TestNewStoreAndLoadDefault(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", cfg.SchemaVersion)
	}

	// Verify split_tunnel.json and bootstrap files were created
	if _, err := os.Stat(filepath.Join(tempDir, "split_tunnel.json")); err != nil {
		t.Errorf("split_tunnel.json was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "split_tunnel_bootstrap.json")); err != nil {
		t.Errorf("split_tunnel_bootstrap.json was not created: %v", err)
	}
}

func TestNewStoreWithEmptyBaseDir(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore(\"\") failed: %v", err)
	}
	if store == nil || store.baseDir == "" {
		t.Errorf("expected non-empty store baseDir")
	}
}

func TestDefaultBaseDir(t *testing.T) {
	dir, err := DefaultBaseDir()
	if err != nil {
		t.Fatalf("DefaultBaseDir failed: %v", err)
	}
	if !strings.HasSuffix(dir, "PWDTT") {
		t.Errorf("expected path ending in PWDTT, got %s", dir)
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	cfg, _ := store.Load()
	cfg.Mode = "exclude"
	cfg.Apps = []AppConfig{
		{Name: "Firefox", ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`, MatchChildren: true},
	}

	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify in-memory config
	inMem := store.GetConfig()
	if inMem.Mode != "exclude" {
		t.Errorf("expected in-memory mode 'exclude', got '%s'", inMem.Mode)
	}
	if len(inMem.Apps) != 1 || inMem.Apps[0].Name != "Firefox" {
		t.Errorf("unexpected in-memory apps: %+v", inMem.Apps)
	}

	// Reload from new Store instance
	store2, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore 2 failed: %v", err)
	}

	loaded, err := store2.Load()
	if err != nil {
		t.Fatalf("Load 2 failed: %v", err)
	}

	if loaded.Mode != "exclude" {
		t.Errorf("expected loaded mode 'exclude', got '%s'", loaded.Mode)
	}
	if len(loaded.Apps) != 1 || loaded.Apps[0].Name != "Firefox" {
		t.Errorf("unexpected loaded apps: %+v", loaded.Apps)
	}
}

func TestSetConfigPathVariants(t *testing.T) {
	baseDir := t.TempDir()
	oneDriveDir := filepath.Join(t.TempDir(), "OneDrive", "PWDTT")

	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	cfg, _ := store.Load()
	cfg.Apps = []AppConfig{
		{Name: "TestApp", ExePath: "C:\\Test\\App.exe", MatchChildren: false},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 1. Same path test (no-op)
	currentPath := store.GetConfigPath()
	if err := store.SetConfigPath(currentPath, true); err != nil {
		t.Errorf("SetConfigPath to same path failed: %v", err)
	}

	// 2. Trailing slash path test
	folderWithSlash := oneDriveDir + string(filepath.Separator)
	if err := store.SetConfigPath(folderWithSlash, false); err != nil {
		t.Fatalf("SetConfigPath with trailing slash failed: %v", err)
	}
	expectedFile := filepath.Join(oneDriveDir, "split_tunnel.json")
	if store.GetConfigPath() != expectedFile {
		t.Errorf("expected %s, got %s", expectedFile, store.GetConfigPath())
	}

	// 3. Move existing file when old file exists
	newTarget := filepath.Join(t.TempDir(), "moved_split_tunnel.json")
	if err := store.SetConfigPath(newTarget, true); err != nil {
		t.Fatalf("SetConfigPath move existing failed: %v", err)
	}
	if store.GetConfigPath() != newTarget {
		t.Errorf("expected %s, got %s", newTarget, store.GetConfigPath())
	}

	// 4. Move existing file when old file does not exist
	_ = os.Remove(newTarget)
	newTarget2 := filepath.Join(t.TempDir(), "moved2_split_tunnel.json")
	if err := store.SetConfigPath(newTarget2, true); err != nil {
		t.Fatalf("SetConfigPath move non-existent old file failed: %v", err)
	}
	if _, err := os.Stat(newTarget2); err != nil {
		t.Errorf("new target should have been created: %v", err)
	}
}

func TestBootstrapCorruptedOrEmpty(t *testing.T) {
	baseDir := t.TempDir()

	// Write invalid JSON in bootstrap file
	bootFile := filepath.Join(baseDir, "split_tunnel_bootstrap.json")
	if err := os.WriteFile(bootFile, []byte("{invalid boot json"), 0644); err != nil {
		t.Fatalf("failed to create corrupt bootstrap: %v", err)
	}

	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load with corrupt bootstrap failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}
}

func TestGetConfigNilAppsHandling(t *testing.T) {
	store := &Store{
		config: Config{
			SchemaVersion: 1,
			Apps:          nil,
		},
	}

	cfg := store.GetConfig()
	if cfg.Apps == nil {
		t.Errorf("expected non-nil Apps slice from GetConfig()")
	}
}

func TestLastKnownGoodFallback(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	cfg, _ := store.Load()
	cfg.Mode = "include"
	cfg.Apps = []AppConfig{{Name: "GoodApp", ExePath: "C:\\Good.exe"}}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Corrupt main split_tunnel.json
	mainPath := store.GetConfigPath()
	if err := os.WriteFile(mainPath, []byte("invalid json {{"), 0644); err != nil {
		t.Fatalf("failed to corrupt main file: %v", err)
	}

	// Re-load: store should recover from last-known-good
	recovered, err := store.Load()
	if err != nil {
		t.Fatalf("Load recovery failed: %v", err)
	}

	if len(recovered.Apps) != 1 || recovered.Apps[0].Name != "GoodApp" {
		t.Errorf("failed to recover from LKG, got: %+v", recovered)
	}

	// Verify main file was overwritten with valid content
	data, _ := os.ReadFile(mainPath)
	var restored Config
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Errorf("main file was not restored with valid JSON: %v", err)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "sub", "test.txt")

	data := []byte("hello atomic")
	if err := atomicWriteFile(targetFile, data, 0644); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	read, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(read) != string(data) {
		t.Errorf("expected %s, got %s", string(data), string(read))
	}
}

func TestConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	_, _ = store.Load()

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				switch (id + j) % 4 {
				case 0:
					_ = store.Save(&Config{
						SchemaVersion:    1,
						Enabled:          true,
						Mode:             "include",
						KillSwitch:       "fail_closed",
						WFPCrashMode:     "dynamic",
						SystemDNS:        "tunnel_dns",
						PortableEnvPaths: true,
						Apps:             []AppConfig{{Name: "ConcurrentApp", ExePath: "C:\\App.exe"}},
					})
				case 1:
					_ = store.GetConfig()
				case 2:
					_, _ = store.Load()
				case 3:
					_ = store.GetConfigPath()
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Save nil config
	if err := store.Save(nil); err == nil {
		t.Errorf("expected error when saving nil config")
	}

	// Set invalid empty config path
	if err := store.SetConfigPath("   ", false); err == nil {
		t.Errorf("expected error when setting empty config path")
	}

	// Fold/expand exported helpers
	if FoldPath("") != "" || ExpandPath("") != "" {
		t.Errorf("expected empty string handling")
	}
}
