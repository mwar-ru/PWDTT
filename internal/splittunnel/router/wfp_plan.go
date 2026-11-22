package router

import (
	"sort"
	"strings"

	"pwdtt/internal/splittunnel/appmgr"
)

// WFPLayer identifies which Windows Filtering Platform ALE layer a filter targets.
type WFPLayer string

const (
	LayerALEAuthConnectV4 WFPLayer = "ale_auth_connect_v4"
	LayerALEAuthConnectV6 WFPLayer = "ale_auth_connect_v6"
)

// WFPAction identifies the terminating filter action.
type WFPAction string

const (
	ActionBlock  WFPAction = "block"
	ActionPermit WFPAction = "permit"
)

// WFPGuardConfig describes inputs needed to build the active WFP Kill Switch filter set.
type WFPGuardConfig struct {
	Enabled       bool
	Mode          string   // "global", "include", "exclude"
	FailClosed    bool     // true = block selected apps when tunnel is down
	CrashMode     string   // "dynamic" or "persistent_guard"
	TunnelHasIPv6 bool     // false = block IPv6 for VPN-routed apps even while tunnel is up
	SelfExePath   string   // PWDTT binary path (never blocked — anti-loop)
	AppPaths      []string // Canonical or raw Windows EXE paths of configured apps
}

// WFPFilterSpec is a declarative, OS-independent description of a single WFP filter rule.
type WFPFilterSpec struct {
	Name       string
	Layer      WFPLayer
	Action     WFPAction
	AppPath    string // Canonical Windows EXE path converted to ALE_APP_ID on Windows
	Weight     uint8
	Persistent bool
}

// WFPGuard manages Windows Filtering Platform Kill Switch and IPv6 leak rules.
type WFPGuard interface {
	Apply(cfg WFPGuardConfig, tunnelUp bool) error
	Clear() error
	ResetStale() error
	Close() error
	ActiveFilterCount() int
}

// BuildWFPFilterPlan computes the exact set of user-mode WFP ALE_AUTH_CONNECT filters
// required for the current split-tunneling configuration and tunnel state.
//
// Important architecture invariant (ADR-002):
// ALE_AUTH_CONNECT executes BEFORE WinDivert OUTBOUND_IPPACKET. Therefore:
//   - When tunnelUp == true: we NEVER install IPv4 BLOCK filters for VPN apps (WinDivert
//     intercepts and re-injects IPv4 into Wintun). We only install IPv6 BLOCK filters if
//     the WireGuard tunnel is IPv4-only to prevent silent IPv6 leaks.
//   - When tunnelUp == false and FailClosed == true: we install V4 + V6 BLOCK filters for
//     selected apps (in "include" mode) so no packets leak over the physical NIC while
//     unselected apps keep working directly.
func BuildWFPFilterPlan(cfg WFPGuardConfig, tunnelUp bool) []WFPFilterSpec {
	if !cfg.Enabled {
		return nil
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "include"
	}
	persistent := strings.ToLower(strings.TrimSpace(cfg.CrashMode)) == "persistent_guard"

	// Deduplicate and sanitize app paths while strictly excluding PWDTT itself (anti-loop)
	selfNorm := appmgr.NormalizeWindowsPath(cfg.SelfExePath)
	seen := make(map[string]struct{}, len(cfg.AppPaths))
	apps := make([]string, 0, len(cfg.AppPaths))
	for _, raw := range cfg.AppPaths {
		norm := appmgr.NormalizeWindowsPath(raw)
		if norm == "" {
			continue
		}
		base := appmgr.BaseExeName(norm)
		if (selfNorm != "" && norm == selfNorm) || base == "pwdtt.exe" || base == "mwpwdtt_windows.exe" || base == "wg-turn-client.exe" {
			continue
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		apps = append(apps, norm)
	}
	sort.Strings(apps)

	var specs []WFPFilterSpec

	// Case 1: Tunnel is UP -> only guard against IPv6 leaks on IPv4-only tunnels.
	if tunnelUp {
		if !cfg.TunnelHasIPv6 && mode == "include" {
			for _, appPath := range apps {
				specs = append(specs, WFPFilterSpec{
					Name:       "PWDTT IPv6 Leak Guard: " + appmgr.BaseExeName(appPath),
					Layer:      LayerALEAuthConnectV6,
					Action:     ActionBlock,
					AppPath:    appPath,
					Weight:     10,
					Persistent: persistent,
				})
			}
		}
		return specs
	}

	// Case 2: Tunnel is DOWN / Reconnecting / Error.
	if !cfg.FailClosed {
		return nil
	}

	switch mode {
	case "include":
		// Block ONLY selected VPN apps on both IPv4 and IPv6; other apps remain direct.
		for _, appPath := range apps {
			base := appmgr.BaseExeName(appPath)
			specs = append(specs,
				WFPFilterSpec{
					Name:       "PWDTT KillSwitch V4: " + base,
					Layer:      LayerALEAuthConnectV4,
					Action:     ActionBlock,
					AppPath:    appPath,
					Weight:     12,
					Persistent: persistent,
				},
				WFPFilterSpec{
					Name:       "PWDTT KillSwitch V6: " + base,
					Layer:      LayerALEAuthConnectV6,
					Action:     ActionBlock,
					AppPath:    appPath,
					Weight:     12,
					Persistent: persistent,
				},
			)
		}
	case "global", "exclude":
		// Always permit PWDTT itself at highest weight so WireGuard/VK TURN can reconnect.
		if selfNorm != "" {
			specs = append(specs,
				WFPFilterSpec{
					Name:       "PWDTT Anti-Loop Permit V4",
					Layer:      LayerALEAuthConnectV4,
					Action:     ActionPermit,
					AppPath:    selfNorm,
					Weight:     15,
					Persistent: persistent,
				},
				WFPFilterSpec{
					Name:       "PWDTT Anti-Loop Permit V6",
					Layer:      LayerALEAuthConnectV6,
					Action:     ActionPermit,
					AppPath:    selfNorm,
					Weight:     15,
					Persistent: persistent,
				},
			)
		}
		// In exclude mode, explicitly permit excluded apps so they continue direct during outage.
		if mode == "exclude" {
			for _, appPath := range apps {
				base := appmgr.BaseExeName(appPath)
				specs = append(specs,
					WFPFilterSpec{
						Name:       "PWDTT Exclude Permit V4: " + base,
						Layer:      LayerALEAuthConnectV4,
						Action:     ActionPermit,
						AppPath:    appPath,
						Weight:     14,
						Persistent: persistent,
					},
					WFPFilterSpec{
						Name:       "PWDTT Exclude Permit V6: " + base,
						Layer:      LayerALEAuthConnectV6,
						Action:     ActionPermit,
						AppPath:    appPath,
						Weight:     14,
						Persistent: persistent,
					},
				)
			}
		}
		// Default-deny fallback for all remaining apps while tunnel is down (empty AppPath = match all).
		specs = append(specs,
			WFPFilterSpec{
				Name:       "PWDTT KillSwitch Global Block V4",
				Layer:      LayerALEAuthConnectV4,
				Action:     ActionBlock,
				AppPath:    "",
				Weight:     10,
				Persistent: persistent,
			},
			WFPFilterSpec{
				Name:       "PWDTT KillSwitch Global Block V6",
				Layer:      LayerALEAuthConnectV6,
				Action:     ActionBlock,
				AppPath:    "",
				Weight:     10,
				Persistent: persistent,
			},
		)
	}

	return specs
}
