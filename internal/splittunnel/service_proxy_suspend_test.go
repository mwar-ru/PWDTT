package splittunnel

import (
	"context"
	"testing"

	"pwdtt/internal/splittunnel/router"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

type proxySuspendFakeRouter struct {
	starts  int
	stops   int
	running bool
}

func (f *proxySuspendFakeRouter) Start(_ context.Context) error {
	f.starts++
	f.running = true
	return nil
}

func (f *proxySuspendFakeRouter) Stop() error {
	f.stops++
	f.running = false
	return nil
}

func (f *proxySuspendFakeRouter) ApplyConfig(_ settings.Config) error { return nil }
func (f *proxySuspendFakeRouter) OnTunnelEvent(_ vpn.TunnelEvent)     {}
func (f *proxySuspendFakeRouter) Stats() router.RouterStats {
	state := "stopped"
	if f.running {
		state = "running"
	}
	return router.RouterStats{State: router.RouterState(state)}
}

func TestServiceProxySuspendAndResume(t *testing.T) {
	fake := &proxySuspendFakeRouter{}
	svc := NewServiceWithDeps(nil, nil, nil, fake, true)

	activeCfg := settings.Config{Enabled: true, Mode: "include"}

	svc.mu.Lock()
	svc.reconcileRouterLocked(activeCfg)
	svc.mu.Unlock()
	if !svc.started || fake.starts != 1 {
		t.Fatalf("expected router started once, got started=%v starts=%d", svc.started, fake.starts)
	}

	if err := svc.SetProxySuspended(true); err != nil {
		t.Fatalf("SetProxySuspended(true) failed: %v", err)
	}
	if svc.started || fake.stops != 1 {
		t.Fatalf("expected router stopped on suspend, got started=%v stops=%d", svc.started, fake.stops)
	}

	// Reconciling an enabled config while suspended must NOT start the router.
	svc.mu.Lock()
	svc.reconcileRouterLocked(activeCfg)
	svc.mu.Unlock()
	if svc.started || fake.starts != 1 {
		t.Fatalf("router must remain stopped while proxySuspended, got started=%v starts=%d", svc.started, fake.starts)
	}

	// Resuming proxy mode must restore and restart the router automatically.
	if err := svc.SetProxySuspended(false); err != nil {
		t.Fatalf("SetProxySuspended(false) failed: %v", err)
	}
	if !svc.started || fake.starts != 2 {
		t.Fatalf("expected router restarted after resume, got started=%v starts=%d", svc.started, fake.starts)
	}
}
