package appmgr

import (
	"sync"
	"testing"
	"time"

	"pwdtt/internal/splittunnel/settings"
)

type mockProcessBackend struct {
	mu    sync.RWMutex
	procs map[uint32]rawProcessInfo
}

func (m *mockProcessBackend) LookupProcess(pid uint32) (rawProcessInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.procs[pid]
	if !ok {
		return rawProcessInfo{}, ErrProcessNotFound
	}
	return p, nil
}

func (m *mockProcessBackend) ListProcesses() ([]rawProcessInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]rawProcessInfo, 0, len(m.procs))
	for _, p := range m.procs {
		out = append(out, p)
	}
	return out, nil
}

func TestProcessResolverBrowserParentInheritance(t *testing.T) {
	backend := &mockProcessBackend{
		procs: map[uint32]rawProcessInfo{
			100: {PID: 100, PPID: 4, CreationTime: 1000, ExePath: `C:\Windows\explorer.exe`},
			200: {PID: 200, PPID: 100, CreationTime: 2000, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
			201: {PID: 201, PPID: 200, CreationTime: 2100, ExePath: `C:\Program Files\Mozilla Firefox\plugin-container.exe`},
		},
	}

	resolver := NewProcessResolverWithBackend(backend)
	info, err := resolver.Resolve(201)
	if err != nil {
		t.Fatalf("Resolve(201) error: %v", err)
	}
	if info.ExePath != `c:\program files\mozilla firefox\plugin-container.exe` {
		t.Fatalf("unexpected ExePath: %s", info.ExePath)
	}
	if len(info.ParentChain) != 2 || info.ParentChain[0] != `c:\program files\mozilla firefox\firefox.exe` {
		t.Fatalf("unexpected ParentChain: %v", info.ParentChain)
	}

	// End-to-end check with PolicyEngine
	engine := NewPolicyEngineWithEnv(settings.Config{
		Enabled: true,
		Mode:    "include",
		Apps: []settings.AppConfig{
			{Name: "Firefox", ExePath: `%ProgramFiles%\Mozilla Firefox\firefox.exe`, MatchChildren: true},
		},
	}, `C:\PWDTT\pwdtt.exe`, mockWinEnv)

	if got := engine.Evaluate(info); got != DecisionVPN {
		t.Fatalf("expected Firefox child to route via VPN, got %s", got)
	}
}

func TestProcessResolverPIDReuseCreationTimeGuard(t *testing.T) {
	// Scenario: child (PID 500, created at t=2000) had PPID=300 (original parent exited).
	// Later, Windows reused PID 300 for Firefox at t=9000 (AFTER child was created!).
	// Child MUST NOT inherit Firefox's VPN rule.
	backend := &mockProcessBackend{
		procs: map[uint32]rawProcessInfo{
			300: {PID: 300, PPID: 4, CreationTime: 9000, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
			500: {PID: 500, PPID: 300, CreationTime: 2000, ExePath: `C:\Tools\unrelated.exe`},
		},
	}

	resolver := NewProcessResolverWithBackend(backend)
	info, err := resolver.Resolve(500)
	if err != nil {
		t.Fatalf("Resolve(500) error: %v", err)
	}
	if len(info.ParentChain) != 0 {
		t.Fatalf("PID-reuse guard failed: inherited newer reused parent %v", info.ParentChain)
	}
}

func TestProcessResolverCycleGuardAndCacheExpiry(t *testing.T) {
	backend := &mockProcessBackend{
		procs: map[uint32]rawProcessInfo{
			10: {PID: 10, PPID: 20, CreationTime: 1000, ExePath: `C:\A\a.exe`},
			20: {PID: 20, PPID: 10, CreationTime: 1000, ExePath: `C:\B\b.exe`},
		},
	}

	now := time.Now()
	resolver := NewProcessResolverWithBackend(backend)
	resolver.nowFn = func() time.Time { return now }

	info, err := resolver.Resolve(10)
	if err != nil {
		t.Fatalf("Resolve(10) error: %v", err)
	}
	if len(info.ParentChain) != 1 || info.ParentChain[0] != `c:\b\b.exe` {
		t.Fatalf("unexpected cyclic parent chain: %v", info.ParentChain)
	}

	// Update backend PID 10 -> should stay cached until TTL expires or Invalidate called
	backend.mu.Lock()
	backend.procs[10] = rawProcessInfo{PID: 10, PPID: 0, CreationTime: 5000, ExePath: `C:\New\new.exe`}
	backend.mu.Unlock()

	cached, _ := resolver.Resolve(10)
	if cached.ExePath != `c:\a\a.exe` {
		t.Fatalf("expected cached value before TTL, got %s", cached.ExePath)
	}

	now = now.Add(defaultResolverTTL + time.Second)
	refreshed, err := resolver.Resolve(10)
	if err != nil || refreshed.ExePath != `c:\new\new.exe` {
		t.Fatalf("expected refreshed path after TTL, got %q (err=%v)", refreshed.ExePath, err)
	}
}

func TestProcessResolverListRunningAppsGrouping(t *testing.T) {
	backend := &mockProcessBackend{
		procs: map[uint32]rawProcessInfo{
			101: {PID: 101, ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
			102: {PID: 102, ExePath: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
			103: {PID: 103, ExePath: `c:\program files\google\chrome\application\CHROME.EXE`},
			201: {PID: 201, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
			202: {PID: 202, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
			999: {PID: 999, ExePath: ``}, // Unreadable PPL process
		},
	}

	resolver := NewProcessResolverWithBackend(backend)
	apps, err := resolver.ListRunningApps()
	if err != nil {
		t.Fatalf("ListRunningApps error: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 deduplicated apps, got %d: %+v", len(apps), apps)
	}
	if apps[0].Name != "Chrome" || apps[0].ProcessCount != 3 {
		t.Fatalf("unexpected first app: %+v", apps[0])
	}
	if apps[1].Name != "Firefox" || apps[1].ProcessCount != 2 {
		t.Fatalf("unexpected second app: %+v", apps[1])
	}
}

func TestProcessResolverConcurrentAccess(t *testing.T) {
	backend := &mockProcessBackend{
		procs: map[uint32]rawProcessInfo{
			10: {PID: 10, PPID: 4, CreationTime: 1000, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
			11: {PID: 11, PPID: 10, CreationTime: 1100, ExePath: `C:\Program Files\Mozilla Firefox\firefox.exe`},
		},
	}
	resolver := NewProcessResolverWithBackend(backend)

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := resolver.Resolve(11); err != nil {
					t.Errorf("Resolve(11): %v", err)
				}
				if id%4 == 0 && j%10 == 0 {
					resolver.Invalidate(11)
				}
			}
		}(i)
	}
	wg.Wait()
}
