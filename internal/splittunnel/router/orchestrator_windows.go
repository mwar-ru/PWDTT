//go:build windows

package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

// WindowsTrafficRouter orchestrates WFP Kill Switch + WinDivert SOCKET/NETWORK + NAT/Staging + DNS Guard.
type WindowsTrafficRouter struct {
	mu           sync.Mutex
	dllDir       string
	expectedHash map[string]string
	selfExePath  string

	resolver *appmgr.ProcessResolver
	wfp      WFPGuard
	nat      *NATTable
	staging  *StagingQueue
	dnsGuard *DNSGuard

	dll     *WinDivertDLL
	sockets *SocketTable
	tracker *SocketTracker
	engine  *networkEngine

	cancel   context.CancelFunc
	cfg      settings.Config
	policy   *appmgr.PolicyEngine
	topology *vpn.NetworkTopology
	tunnelUp bool
	state    RouterState
	lastErr  string
}

// NewWindowsTrafficRouter builds a TrafficRouter for Windows 11 x64.
func NewWindowsTrafficRouter(dllDir string, expectedHashes map[string]string, resolver *appmgr.ProcessResolver) *WindowsTrafficRouter {
	if resolver == nil {
		resolver = appmgr.NewWindowsProcessResolver()
	}
	return &WindowsTrafficRouter{
		dllDir:       dllDir,
		expectedHash: expectedHashes,
		selfExePath:  appmgr.SelfExecutablePath(),
		resolver:     resolver,
		wfp:          NewWindowsWFPGuard(),
		nat:          NewNATTable(),
		staging:      NewStagingQueue(),
		dnsGuard:     NewDNSGuard(),
		cfg:          settings.DefaultConfig(),
		state:        RouterStopped,
	}
}

// Start cleans stale WFP rules, loads WinDivert 2.2, and starts SOCKET + NETWORK + DNS Guard engines.
func (r *WindowsTrafficRouter) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == RouterRunning {
		return nil
	}

	// 1. Clean any orphaned persistent WFP filters from a previous hard crash (Risk R02)
	_ = r.wfp.ResetStale()

	// 2. Load integrity-verified WinDivert 2.2
	dll, err := LoadWinDivert(r.dllDir, r.expectedHash)
	if err != nil {
		r.state = RouterError
		r.lastErr = err.Error()
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	var eng *networkEngine
	sockTable := NewSocketTable(func(key SocketKey, pid uint32) {
		if eng != nil {
			eng.OnSocketMapped(key, pid)
		}
	})
	tracker := NewSocketTracker(dll, sockTable)
	eng = newNetworkEngine(dll, tracker, r.resolver, r.nat, r.staging)
	eng.SetDNSHook(newDNSPacketHook(r.dnsGuard, eng))

	if r.policy == nil {
		r.policy = appmgr.NewPolicyEngine(r.cfg, r.selfExePath)
	}
	eng.SetPolicy(r.policy)
	eng.SetTopology(r.topology, r.tunnelUp)

	if err := tracker.Start(runCtx); err != nil {
		cancel()
		r.state = RouterError
		r.lastErr = err.Error()
		return fmt.Errorf("start WinDivert SOCKET tracker: %w", err)
	}
	if err := eng.Start(runCtx); err != nil {
		_ = tracker.Stop()
		cancel()
		r.state = RouterError
		r.lastErr = err.Error()
		return fmt.Errorf("start WinDivert NETWORK engine: %w", err)
	}

	go r.dnsGuard.RunSweep(runCtx, 5*time.Second)

	r.dll = dll
	r.sockets = sockTable
	r.tracker = tracker
	r.engine = eng
	r.cancel = cancel
	r.state = RouterRunning
	r.lastErr = ""

	r.syncWFPLocked()
	return nil
}

// Stop gracefully stops WinDivert handles, clears NAT/staging/DNS state, and closes WFP.
func (r *WindowsTrafficRouter) Stop() error {
	r.mu.Lock()
	cancel := r.cancel
	eng := r.engine
	tracker := r.tracker
	r.cancel = nil
	r.engine = nil
	r.tracker = nil
	r.state = RouterStopped
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if eng != nil {
		_ = eng.Stop()
	}
	if tracker != nil {
		_ = tracker.Stop()
	}
	r.nat.Clear()
	r.staging.Clear()
	r.dnsGuard.Clear()
	return r.wfp.Close()
}

// ApplyConfig compiles a fresh PolicyEngine and atomically updates both WinDivert and WFP Kill Switch.
func (r *WindowsTrafficRouter) ApplyConfig(cfg settings.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cfg = cfg
	r.policy = appmgr.NewPolicyEngine(cfg, r.selfExePath)
	if r.engine != nil {
		r.engine.SetPolicy(r.policy)
	}
	r.nat.Clear()
	r.dnsGuard.Clear()
	r.syncWFPLocked()
	return nil
}

// OnTunnelEvent reacts to upstream PWDTT lifecycle events (Starting/Up/Reconnecting/Down/Error).
func (r *WindowsTrafficRouter) OnTunnelEvent(ev vpn.TunnelEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch ev.State {
	case vpn.StateUp:
		r.tunnelUp = true
		if ev.Topology != nil {
			cp := *ev.Topology
			r.topology = &cp
		}
	case vpn.StateDown, vpn.StateReconnecting, vpn.StateError, vpn.StateStarting:
		r.tunnelUp = false
		if ev.State == vpn.StateDown {
			r.topology = nil
		}
		r.nat.Clear()
		r.staging.Clear()
		r.dnsGuard.Clear()
	}

	if r.engine != nil {
		r.engine.SetTopology(r.topology, r.tunnelUp)
	}
	r.syncWFPLocked()
}

func (r *WindowsTrafficRouter) syncWFPLocked() {
	if r.wfp == nil || r.policy == nil {
		return
	}
	hasIPv6 := r.topology != nil && r.topology.HasIPv6Tunnel()
	wfpCfg := WFPGuardConfig{
		Enabled:       r.cfg.Enabled,
		Mode:          r.policy.Mode(),
		FailClosed:    r.policy.KillSwitchFailClosed(),
		CrashMode:     r.cfg.WFPCrashMode,
		TunnelHasIPv6: hasIPv6,
		SelfExePath:   r.selfExePath,
		AppPaths:      r.policy.MatchedAppPaths(),
	}
	if err := r.wfp.Apply(wfpCfg, r.tunnelUp); err != nil {
		r.lastErr = "WFP apply warning: " + err.Error()
	}
}

// Stats returns a thread-safe telemetry snapshot for UI and diagnostics.
func (r *WindowsTrafficRouter) Stats() RouterStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	st := RouterStats{
		State:             r.state,
		Mode:              r.cfg.Mode,
		TunnelUp:          r.tunnelUp,
		ActiveNATSessions: r.nat.Len(),
		ActiveWFPFilters:  r.wfp.ActiveFilterCount(),
		LastError:         r.lastErr,
	}
	if r.engine != nil {
		st.PacketsToVPN = r.engine.packetsToVPN.Load()
		st.PacketsDirect = r.engine.packetsDirect.Load()
		st.PacketsDroppedFailClosed = r.engine.packetsDropped.Load()
		st.DNSRedirected = r.engine.dnsRedirected.Load()
		st.StagingQueued = r.engine.stagingQueued.Load()
		st.StagingExpired = r.engine.stagingExpired.Load()
	}
	return st
}
