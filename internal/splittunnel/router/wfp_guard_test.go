package router

import "testing"

func TestBuildWFPFilterPlanIncludeModeTunnelDownFailClosed(t *testing.T) {
	cfg := WFPGuardConfig{
		Enabled:       true,
		Mode:          "include",
		FailClosed:    true,
		CrashMode:     "dynamic",
		TunnelHasIPv6: false,
		SelfExePath:   `C:\Apps\PWDTT\pwdtt.exe`,
		AppPaths: []string{
			`C:\Program Files\Mozilla Firefox\firefox.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Apps\PWDTT\pwdtt.exe`, // must be stripped by anti-loop!
		},
	}

	specs := BuildWFPFilterPlan(cfg, false)
	// 2 apps * (V4 + V6) = 4 BLOCK filters; pwdtt.exe MUST NOT be blocked
	if len(specs) != 4 {
		t.Fatalf("expected 4 kill-switch filters (2 apps x V4/V6), got %d: %+v", len(specs), specs)
	}
	for _, s := range specs {
		if s.Action != ActionBlock {
			t.Fatalf("expected ActionBlock, got %+v", s)
		}
		if s.Persistent {
			t.Fatalf("expected dynamic (Persistent=false), got %+v", s)
		}
		if s.AppPath == `c:\apps\pwdtt\pwdtt.exe` {
			t.Fatalf("anti-loop violated: pwdtt.exe present in block filter")
		}
	}
}

func TestBuildWFPFilterPlanIncludeModeTunnelUpIPv6OnlyGuard(t *testing.T) {
	cfg := WFPGuardConfig{
		Enabled:       true,
		Mode:          "include",
		FailClosed:    true,
		CrashMode:     "persistent_guard",
		TunnelHasIPv6: false,
		SelfExePath:   `C:\Apps\PWDTT\pwdtt.exe`,
		AppPaths: []string{
			`C:\Program Files\Mozilla Firefox\firefox.exe`,
		},
	}

	// Tunnel UP + IPv4-only -> MUST NOT block IPv4 (WinDivert handles IPv4), MUST block IPv6!
	specs := BuildWFPFilterPlan(cfg, true)
	if len(specs) != 1 {
		t.Fatalf("expected 1 IPv6-only leak guard filter, got %d: %+v", len(specs), specs)
	}
	if specs[0].Layer != LayerALEAuthConnectV6 || specs[0].Action != ActionBlock || !specs[0].Persistent {
		t.Fatalf("unexpected IPv6 leak guard filter: %+v", specs[0])
	}

	// Dual-stack tunnel (TunnelHasIPv6 = true) -> 0 block filters when tunnel is up
	cfg.TunnelHasIPv6 = true
	if specsDual := BuildWFPFilterPlan(cfg, true); len(specsDual) != 0 {
		t.Fatalf("expected 0 filters on dual-stack tunnel up, got %d", len(specsDual))
	}
}

func TestBuildWFPFilterPlanFailOpenAndExcludeMode(t *testing.T) {
	// Fail-open -> zero filters when tunnel is down
	failOpenSpecs := BuildWFPFilterPlan(WFPGuardConfig{
		Enabled:    true,
		Mode:       "include",
		FailClosed: false,
		AppPaths:   []string{`C:\Program Files\Mozilla Firefox\firefox.exe`},
	}, false)
	if len(failOpenSpecs) != 0 {
		t.Fatalf("expected 0 filters in fail-open mode, got %d", len(failOpenSpecs))
	}

	// Exclude mode + tunnel down + fail_closed:
	// Permit PWDTT (V4/V6) + Permit Chrome (V4/V6) + Global Block (V4/V6) = 6 filters
	excludeSpecs := BuildWFPFilterPlan(WFPGuardConfig{
		Enabled:     true,
		Mode:        "exclude",
		FailClosed:  true,
		SelfExePath: `C:\PWDTT\pwdtt.exe`,
		AppPaths:    []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`},
	}, false)
	if len(excludeSpecs) != 6 {
		t.Fatalf("expected 6 filters in exclude fail-closed mode, got %d: %+v", len(excludeSpecs), excludeSpecs)
	}
}
