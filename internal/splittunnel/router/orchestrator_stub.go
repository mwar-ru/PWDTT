//go:build !windows

package router

import (
	"context"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

type stubTrafficRouter struct {
	mode string
}

// NewWindowsTrafficRouter returns a no-op TrafficRouter on non-Windows platforms.
func NewWindowsTrafficRouter(string, map[string]string, *appmgr.ProcessResolver) TrafficRouter {
	return &stubTrafficRouter{mode: "global"}
}

func (s *stubTrafficRouter) Start(context.Context) error { return nil }
func (s *stubTrafficRouter) Stop() error                 { return nil }
func (s *stubTrafficRouter) ApplyConfig(cfg settings.Config) error {
	s.mode = cfg.Mode
	return nil
}
func (s *stubTrafficRouter) OnTunnelEvent(vpn.TunnelEvent) {}
func (s *stubTrafficRouter) Stats() RouterStats {
	return RouterStats{State: RouterStopped, Mode: s.mode}
}
