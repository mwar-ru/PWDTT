package splittunnel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/router"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

// StatusSnapshot is the live diagnostic state returned to the Wails frontend.
type StatusSnapshot struct {
	Supported     bool                 `json:"supported"`
	Running       bool                 `json:"running"`
	ConfigPath    string               `json:"config_path"`
	TunnelState   vpn.TunnelState      `json:"tunnel_state"`
	Topology      *vpn.NetworkTopology `json:"topology,omitempty"`
	Router        router.RouterStats   `json:"router"`
	CompetingVPNs []vpn.InterfaceInfo  `json:"competing_vpns,omitempty"`
	Warning       string               `json:"warning,omitempty"`
}

// Service is the Wails-bound controller for Per-App Split Tunneling.
type Service struct {
	mu sync.Mutex

	ctx       context.Context
	runCancel context.CancelFunc

	store       *settings.Store
	resolver    *appmgr.ProcessResolver
	adapter     *vpn.PWDTTAdapter
	router      router.TrafficRouter
	unsubscribe func()

	supported      bool
	started        bool
	proxySuspended bool
	lastCfg        settings.Config
	lastWarn       string
}

// NewService creates the platform-appropriate SplitTunnelService.
func NewService() *Service {
	return newPlatformService()
}

// NewServiceWithDeps creates a Service with injected dependencies for unit testing.
func NewServiceWithDeps(
	store *settings.Store,
	resolver *appmgr.ProcessResolver,
	adapter *vpn.PWDTTAdapter,
	tr router.TrafficRouter,
	supported bool,
) *Service {
	return &Service{
		store:     store,
		resolver:  resolver,
		adapter:   adapter,
		router:    tr,
		supported: supported,
	}
}

// Startup binds tunnel hooks, loads saved split-tunnel settings, and starts the router when needed.
func (s *Service) Startup(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx = ctx

	cfg := settings.DefaultConfig()
	if s.store != nil {
		if loaded, err := s.store.Load(); err == nil && loaded != nil {
			cfg = *loaded
		} else if err != nil {
			s.lastWarn = fmt.Sprintf("load split tunnel config: %v", err)
		}
	}

	if s.adapter != nil {
		s.adapter.SetSplitTunnelMode(cfg.Enabled, cfg.Mode)
		vpn.SetActiveHooks(s.adapter)
		if s.unsubscribe == nil {
			s.unsubscribe = s.adapter.Subscribe(s.onTunnelEvent)
		}
	}

	if s.router != nil {
		if err := s.router.ApplyConfig(cfg); err != nil {
			s.lastWarn = fmt.Sprintf("apply split tunnel config: %v", err)
		}
	}

	s.reconcileRouterLocked(cfg)
}

// Shutdown cleanly tears down the packet router, WFP filters, and tunnel hooks.
func (s *Service) Shutdown(_ context.Context) {
	s.mu.Lock()
	unsub := s.unsubscribe
	s.unsubscribe = nil
	cancel := s.runCancel
	s.runCancel = nil
	s.started = false
	tr := s.router
	hadAdapter := s.adapter != nil
	s.mu.Unlock()

	if unsub != nil {
		unsub()
	}
	if hadAdapter {
		vpn.SetActiveHooks(nil)
	}
	if cancel != nil {
		cancel()
	}
	if tr != nil {
		_ = tr.Stop()
	}
}

// IsSupported reports whether Per-App Split Tunneling is available on this OS (Windows 11).
func (s *Service) IsSupported() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.supported
}

// GetConfig returns the current persisted split-tunnel configuration.
func (s *Service) GetConfig() (settings.Config, error) {
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()

	if store == nil {
		return settings.DefaultConfig(), nil
	}
	loaded, err := store.Load()
	if err != nil {
		return settings.Config{}, err
	}
	if loaded == nil {
		return settings.DefaultConfig(), nil
	}
	return *loaded, nil
}

// SaveConfig validates, persists, and live-applies a new split-tunnel configuration.
func (s *Service) SaveConfig(cfg settings.Config) (settings.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return settings.DefaultConfig(), errors.New("settings store not initialized")
	}
	if err := s.store.Save(&cfg); err != nil {
		return settings.Config{}, err
	}
	saved := s.store.GetConfig()

	if s.adapter != nil {
		s.adapter.SetSplitTunnelMode(saved.Enabled, saved.Mode)
	}
	if s.router != nil {
		if err := s.router.ApplyConfig(saved); err != nil {
			s.lastWarn = err.Error()
		}
	}
	s.reconcileRouterLocked(saved)
	return saved, nil
}

// GetConfigPath returns the active path to split_tunnel.json (default or custom OneDrive path).
func (s *Service) GetConfigPath() string {
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()

	if store == nil {
		return ""
	}
	return store.GetConfigPath()
}

// SetConfigPath updates the bootstrap pointer to a custom split_tunnel.json path (e.g., OneDrive).
// When moveExisting is false and target file already exists, the synced file is preserved and loaded.
func (s *Service) SetConfigPath(newPath string, moveExisting bool) (settings.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return settings.DefaultConfig(), errors.New("settings store not initialized")
	}
	if err := s.store.SetConfigPath(newPath, moveExisting); err != nil {
		return settings.Config{}, err
	}
	loaded := s.store.GetConfig()

	if s.adapter != nil {
		s.adapter.SetSplitTunnelMode(loaded.Enabled, loaded.Mode)
	}
	if s.router != nil {
		if err := s.router.ApplyConfig(loaded); err != nil {
			s.lastWarn = err.Error()
		}
	}
	s.reconcileRouterLocked(loaded)
	return loaded, nil
}

// ListRunningApps returns grouped user applications currently running on the host.
func (s *Service) ListRunningApps() ([]appmgr.RunningApp, error) {
	s.mu.Lock()
	resolver := s.resolver
	s.mu.Unlock()

	if resolver == nil {
		return []appmgr.RunningApp{}, nil
	}
	apps, err := resolver.ListRunningApps()
	if err != nil {
		return nil, err
	}
	if apps == nil {
		return []appmgr.RunningApp{}, nil
	}
	return apps, nil
}

// GetStatus returns a thread-safe snapshot of tunnel topology, competing VPNs, and packet counters.
func (s *Service) GetStatus() StatusSnapshot {
	s.mu.Lock()
	supported := s.supported
	started := s.started
	warn := s.lastWarn
	store := s.store
	adapter := s.adapter
	tr := s.router
	s.mu.Unlock()

	snap := StatusSnapshot{
		Supported:   supported,
		Running:     started,
		TunnelState: vpn.StateDown,
		Warning:     warn,
	}
	if store != nil {
		snap.ConfigPath = store.GetConfigPath()
	}
	if adapter != nil {
		snap.TunnelState = adapter.State()
		topo := adapter.LastTopology()
		snap.Topology = topo
		if topo != nil && len(topo.CompetingVPNs) > 0 {
			snap.CompetingVPNs = append([]vpn.InterfaceInfo(nil), topo.CompetingVPNs...)
		}
	}
	if tr != nil {
		snap.Router = tr.Stats()
		if snap.Warning == "" && snap.Router.LastError != "" {
			snap.Warning = snap.Router.LastError
		}
	}
	return snap
}

// ResetWfpRules performs emergency cleanup of all PWDTT-owned WFP filters.
func (s *Service) ResetWfpRules() error {
	return resetPlatformWfpRules()
}

// PickExecutableDialog opens a native file picker for selecting a Windows .exe application.
func (s *Service) PickExecutableDialog() (string, error) {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		return "", errors.New("wails runtime context not ready")
	}
	return wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
		Title: "Select Application Executable",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Windows Executables (*.exe)", Pattern: "*.exe"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
}

// PickConfigFileDialog opens a native dialog to pick an existing or new split_tunnel.json path (e.g., OneDrive).
func (s *Service) PickConfigFileDialog(existingOnly bool) (string, error) {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		return "", errors.New("wails runtime context not ready")
	}
	filters := []wailsruntime.FileFilter{
		{DisplayName: "JSON Settings (*.json)", Pattern: "*.json"},
	}
	if existingOnly {
		return wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
			Title:   "Select Existing split_tunnel.json",
			Filters: filters,
		})
	}
	return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:           "Choose Location for split_tunnel.json",
		DefaultFilename: "split_tunnel.json",
		Filters:         filters,
	})
}

func (s *Service) onTunnelEvent(ev vpn.TunnelEvent) {
	s.mu.Lock()
	tr := s.router
	s.mu.Unlock()
	if tr != nil {
		tr.OnTunnelEvent(ev)
	}
}

// SetProxySuspended pauses or resumes the split-tunneling router while an external
// SOCKS5 proxy mode (e.g. mihomo) is active so WinDivert/WFP rules cannot conflict.
func (s *Service) SetProxySuspended(suspended bool) error {
	s.mu.Lock()
	s.proxySuspended = suspended
	cfg := s.lastCfg
	if s.store != nil {
		cfg = s.store.GetConfig()
	}
	s.reconcileRouterLocked(cfg)
	shouldResetWfp := suspended && s.supported && s.ctx != nil
	s.mu.Unlock()

	if shouldResetWfp {
		return s.ResetWfpRules()
	}
	return nil
}

func (s *Service) reconcileRouterLocked(cfg settings.Config) {
	s.lastCfg = cfg
	if !s.supported || s.router == nil {
		return
	}
	shouldRun := !s.proxySuspended && cfg.Enabled && cfg.Mode != "global"

	if shouldRun && !s.started {
		baseCtx := s.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		runCtx, cancel := context.WithCancel(baseCtx)
		if err := s.router.Start(runCtx); err != nil {
			cancel()
			s.lastWarn = err.Error()
			return
		}
		s.runCancel = cancel
		s.started = true
		s.lastWarn = ""
		return
	}

	if !shouldRun && s.started {
		if s.runCancel != nil {
			s.runCancel()
			s.runCancel = nil
		}
		if err := s.router.Stop(); err != nil {
			s.lastWarn = err.Error()
		} else {
			s.lastWarn = ""
		}
		s.started = false
	}
}
