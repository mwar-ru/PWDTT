package appmgr

import (
	"sync"
	"testing"

	"pwdtt/internal/splittunnel/settings"
)

func mockWinEnv(key string) string {
	env := map[string]string{
		"ProgramFiles":      `C:\Program Files`,
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"LocalAppData":      `C:\Users\Alice\AppData\Local`,
		"AppData":           `C:\Users\Alice\AppData\Roaming`,
		"UserProfile":       `C:\Users\Alice`,
	}
	return env[key]
}

func TestNormalizeWindowsPathWithEnv(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "standard path with mixed case and forward slashes",
			in:   `C:/Program Files/Mozilla Firefox/FIREFOX.EXE`,
			want: `c:\program files\mozilla firefox\firefox.exe`,
		},
		{
			name: "quoted path with whitespace",
			in:   `  "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"  `,
			want: `c:\program files (x86)\microsoft\edge\application\msedge.exe`,
		},
		{
			name: "win32 extended length prefix",
			in:   `\\?\C:\Program Files\Mozilla Firefox\firefox.exe`,
			want: `c:\program files\mozilla firefox\firefox.exe`,
		},
		{
			name: "nt prefix with dot segments",
			in:   `\??\C:\Program Files\Mozilla Firefox\.\..\Mozilla Firefox\firefox.exe`,
			want: `c:\program files\mozilla firefox\firefox.exe`,
		},
		{
			name: "portable env var expansion",
			in:   `%ProgramFiles%\Mozilla Firefox\firefox.exe`,
			want: `c:\program files\mozilla firefox\firefox.exe`,
		},
		{
			name: "portable localappdata expansion",
			in:   `%LocalAppData%\Programs\Telegram Desktop\Telegram.exe`,
			want: `c:\users\alice\appdata\local\programs\telegram desktop\telegram.exe`,
		},
		{
			name: "unc extended path",
			in:   `\\?\UNC\fileserver\share\App\tool.exe`,
			want: `\\fileserver\share\app\tool.exe`,
		},
		{
			name: "empty input",
			in:   "   ",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeWindowsPathWithEnv(tc.in, mockWinEnv)
			if got != tc.want {
				t.Fatalf("NormalizeWindowsPathWithEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeNTDevicePath(t *testing.T) {
	driveMap := map[string]string{
		`\Device\HarddiskVolume1`:  "D:",
		`\Device\HarddiskVolume10`: "C:",
	}

	got := NormalizeNTDevicePath(`\Device\HarddiskVolume10\Program Files\Mozilla Firefox\firefox.exe`, driveMap)
	want := `c:\program files\mozilla firefox\firefox.exe`
	if got != want {
		t.Fatalf("NormalizeNTDevicePath = %q, want %q", got, want)
	}

	// Ensure Volume1 does not falsely match Volume10 prefix
	got2 := NormalizeNTDevicePath(`\Device\HarddiskVolume1\Games\game.exe`, driveMap)
	want2 := `d:\games\game.exe`
	if got2 != want2 {
		t.Fatalf("NormalizeNTDevicePath (Volume1) = %q, want %q", got2, want2)
	}
}

func TestPolicyEngineIncludeModeBrowsersAndChildren(t *testing.T) {
	cfg := settings.Config{
		SchemaVersion:    1,
		Enabled:          true,
		Mode:             "include",
		KillSwitch:       "fail_closed",
		WFPCrashMode:     "dynamic",
		SystemDNS:        "tunnel_dns",
		PortableEnvPaths: true,
		Apps: []settings.AppConfig{
			{Name: "Firefox", ExePath: `%ProgramFiles%\Mozilla Firefox\firefox.exe`, MatchChildren: true},
			{Name: "Edge", ExePath: `%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe`, MatchChildren: true},
			{Name: "NoChildTool", ExePath: `C:\Tools\parent.exe`, MatchChildren: false},
		},
	}

	engine := NewPolicyEngineWithEnv(cfg, `C:\Apps\PWDTT\pwdtt.exe`, mockWinEnv)

	tests := []struct {
		name string
		proc ProcessInfo
		want RouteDecision
	}{
		{
			name: "Firefox main process -> VPN",
			proc: ProcessInfo{PID: 1001, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
			want: DecisionVPN,
		},
		{
			name: "Firefox socket child process (same exe) -> VPN",
			proc: ProcessInfo{
				PID:         1002,
				ExePath:     `\\?\C:\Program Files\Mozilla Firefox\firefox.exe`,
				ParentChain: []string{`C:\Program Files\Mozilla Firefox\firefox.exe`},
			},
			want: DecisionVPN,
		},
		{
			name: "Firefox helper subprocess (different exe, inherited via MatchChildren=true) -> VPN",
			proc: ProcessInfo{
				PID:         1003,
				ExePath:     `C:\Program Files\Mozilla Firefox\plugin-container.exe`,
				ParentChain: []string{`C:\Program Files\Mozilla Firefox\firefox.exe`},
			},
			want: DecisionVPN,
		},
		{
			name: "Edge NetworkService utility child -> VPN",
			proc: ProcessInfo{
				PID:         2002,
				ExePath:     `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
				ParentChain: []string{`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`},
			},
			want: DecisionVPN,
		},
		{
			name: "Google Chrome (not selected) -> Direct",
			proc: ProcessInfo{PID: 3001, ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
			want: DecisionDirect,
		},
		{
			name: "Child of MatchChildren=false app -> Direct",
			proc: ProcessInfo{
				PID:         4002,
				ExePath:     `C:\Tools\child.exe`,
				ParentChain: []string{`C:\Tools\parent.exe`},
			},
			want: DecisionDirect,
		},
		{
			name: "Other system/user app -> Direct",
			proc: ProcessInfo{PID: 5001, ExePath: `C:\Windows\System32\curl.exe`},
			want: DecisionDirect,
		},
		{
			name: "PWDTT self process -> BypassAntiLoop",
			proc: ProcessInfo{PID: 999, ExePath: `C:\Apps\PWDTT\pwdtt.exe`},
			want: DecisionBypassAntiLoop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := engine.Evaluate(tc.proc)
			if got != tc.want {
				t.Fatalf("Evaluate(%+v) = %s, want %s", tc.proc, got, tc.want)
			}
		})
	}
}

func TestPolicyEngineExcludeAndGlobalModes(t *testing.T) {
	baseApps := []settings.AppConfig{
		{Name: "Chrome", ExePath: `%ProgramFiles%\Google\Chrome\Application\chrome.exe`, MatchChildren: true},
	}

	// Exclude mode: Chrome + children go Direct, everything else goes VPN
	excludeEngine := NewPolicyEngineWithEnv(settings.Config{
		Enabled: true,
		Mode:    "exclude",
		Apps:    baseApps,
	}, `C:\PWDTT\pwdtt.exe`, mockWinEnv)

	if got := excludeEngine.Evaluate(ProcessInfo{ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`}); got != DecisionDirect {
		t.Fatalf("exclude Chrome = %s, want direct", got)
	}
	if got := excludeEngine.Evaluate(ProcessInfo{
		ExePath:     `C:\Program Files\Google\Chrome\Application\chrome_helper.exe`,
		ParentChain: []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`},
	}); got != DecisionDirect {
		t.Fatalf("exclude Chrome child = %s, want direct", got)
	}
	if got := excludeEngine.Evaluate(ProcessInfo{ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`}); got != DecisionVPN {
		t.Fatalf("exclude Firefox = %s, want vpn", got)
	}
	if got := excludeEngine.Evaluate(ProcessInfo{ExePath: `C:\PWDTT\pwdtt.exe`}); got != DecisionBypassAntiLoop {
		t.Fatalf("exclude PWDTT = %s, want bypass_anti_loop", got)
	}

	// Global mode: all normal apps go VPN, PWDTT still bypasses
	globalEngine := NewPolicyEngineWithEnv(settings.Config{
		Enabled: true,
		Mode:    "global",
		Apps:    baseApps,
	}, `C:\PWDTT\pwdtt.exe`, mockWinEnv)

	if got := globalEngine.Evaluate(ProcessInfo{ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`}); got != DecisionVPN {
		t.Fatalf("global Chrome = %s, want vpn", got)
	}
	if got := globalEngine.Evaluate(ProcessInfo{ExePath: `C:\PWDTT\pwdtt.exe`}); got != DecisionBypassAntiLoop {
		t.Fatalf("global PWDTT = %s, want bypass_anti_loop", got)
	}

	// Disabled split tunneling falls back to standard VPN behavior (except anti-loop)
	disabledEngine := NewPolicyEngineWithEnv(settings.Config{
		Enabled: false,
		Mode:    "include",
		Apps:    nil,
	}, `C:\PWDTT\pwdtt.exe`, mockWinEnv)

	if got := disabledEngine.Evaluate(ProcessInfo{ExePath: `C:\Windows\System32\curl.exe`}); got != DecisionVPN {
		t.Fatalf("disabled split tunnel curl = %s, want vpn", got)
	}
}

func TestPolicyEngineAntiLoopEvenIfMisconfigured(t *testing.T) {
	// Even if user mistakenly adds pwdtt.exe to Apps in include mode, anti-loop MUST win
	cfg := settings.Config{
		Enabled: true,
		Mode:    "include",
		Apps: []settings.AppConfig{
			{Name: "PWDTT", ExePath: `C:\CustomFolder\my-pwdtt.exe`, MatchChildren: true},
			{Name: "Firefox", ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`, MatchChildren: true},
		},
	}

	engine := NewPolicyEngineWithEnv(cfg, `C:\CustomFolder\my-pwdtt.exe`, mockWinEnv)

	if got := engine.Evaluate(ProcessInfo{ExePath: `C:\CustomFolder\my-pwdtt.exe`}); got != DecisionBypassAntiLoop || got.UseVPN() {
		t.Fatalf("expected custom self exe to bypass anti-loop, got %s", got)
	}
	if got := engine.Evaluate(ProcessInfo{
		ExePath:     `C:\CustomFolder\helper.exe`,
		ParentChain: []string{`C:\CustomFolder\my-pwdtt.exe`},
	}); got != DecisionBypassAntiLoop {
		t.Fatalf("expected child of self exe to bypass anti-loop, got %s", got)
	}

	paths := engine.MatchedAppPaths()
	if len(paths) != 1 || paths[0] != `c:\program files\mozilla firefox\firefox.exe` {
		t.Fatalf("expected self exe stripped from MatchedAppPaths, got %v", paths)
	}
}

func TestPolicyEngineConcurrentEvaluate(t *testing.T) {
	cfg := settings.Config{
		Enabled: true,
		Mode:    "include",
		Apps: []settings.AppConfig{
			{Name: "Firefox", ExePath: `%ProgramFiles%\Mozilla Firefox\firefox.exe`, MatchChildren: true},
		},
	}
	engine := NewPolicyEngineWithEnv(cfg, `C:\PWDTT\pwdtt.exe`, mockWinEnv)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if id%2 == 0 {
					if !engine.Evaluate(ProcessInfo{ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`}).UseVPN() {
						t.Errorf("expected VPN for Firefox")
					}
				} else {
					if engine.Evaluate(ProcessInfo{ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`}).UseVPN() {
						t.Errorf("expected Direct for Chrome")
					}
				}
			}
		}(i)
	}
	wg.Wait()
}
