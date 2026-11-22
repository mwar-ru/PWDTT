package appmgr

import (
	"os"
	"sort"
	"strings"

	"pwdtt/internal/splittunnel/settings"
)

// RouteDecision describes how network traffic for a process should be routed.
type RouteDecision int

const (
	// DecisionDirect routes traffic directly through the physical NIC.
	DecisionDirect RouteDecision = iota
	// DecisionVPN routes traffic through the PWDTT WireGuard/Wintun tunnel.
	DecisionVPN
	// DecisionBypassAntiLoop forces direct traffic for PWDTT itself to prevent tunneling loops.
	DecisionBypassAntiLoop
)

// String returns a readable identifier for diagnostics and logging.
func (d RouteDecision) String() string {
	switch d {
	case DecisionVPN:
		return "vpn"
	case DecisionBypassAntiLoop:
		return "bypass_anti_loop"
	default:
		return "direct"
	}
}

// UseVPN reports whether traffic with this decision must traverse the VPN tunnel.
func (d RouteDecision) UseVPN() bool {
	return d == DecisionVPN
}

// ProcessInfo represents the identity of a process (and optional parent chain) for policy evaluation.
type ProcessInfo struct {
	PID         uint32
	ExePath     string
	ParentChain []string // Ancestor executable paths (immediate parent first)
}

type compiledRule struct {
	name          string
	canonicalPath string
	matchChildren bool
}

// PolicyEngine is an immutable, goroutine-safe evaluator of per-app split tunneling rules.
type PolicyEngine struct {
	enabled         bool
	mode            string
	killSwitch      string
	ignoreKillSwitchOnDisconnect bool
	systemDNS       string
	selfPath        string
	antiLoopNames   map[string]struct{}
	rulesByPath     map[string]compiledRule
	getenv          func(string) string
}

// NewPolicyEngine compiles a PolicyEngine from settings.Config and the current PWDTT executable path.
func NewPolicyEngine(cfg settings.Config, selfExePath string) *PolicyEngine {
	return NewPolicyEngineWithEnv(cfg, selfExePath, os.Getenv)
}

// NewPolicyEngineWithEnv compiles a PolicyEngine using a custom environment resolver (useful for tests).
func NewPolicyEngineWithEnv(cfg settings.Config, selfExePath string, getenv func(string) string) *PolicyEngine {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	switch mode {
	case "global", "include", "exclude":
	default:
		mode = "include"
	}

	killSwitch := strings.ToLower(strings.TrimSpace(cfg.KillSwitch))
	if killSwitch != "fail_open" {
		killSwitch = "fail_closed"
	}

	systemDNS := strings.ToLower(strings.TrimSpace(cfg.SystemDNS))
	if systemDNS != "system_dns" {
		systemDNS = "tunnel_dns"
	}

	selfNorm := NormalizeWindowsPathWithEnv(selfExePath, getenv)

	antiLoopNames := map[string]struct{}{
		"pwdtt.exe":           {},
		"mwpwdtt_windows.exe": {},
		"wg-turn-client.exe": {},
	}
	if base := BaseExeName(selfNorm); base != "" {
		antiLoopNames[base] = struct{}{}
	}

	rules := make(map[string]compiledRule, len(cfg.Apps))
	for _, app := range cfg.Apps {
		if app.Disabled {
			continue
		}
		normPath := NormalizeWindowsPathWithEnv(app.ExePath, getenv)
		if normPath == "" {
			continue
		}
		// Never allow PWDTT itself into VPN rules (anti-loop invariant)
		if isAntiLoopPath(normPath, selfNorm, antiLoopNames) {
			continue
		}
		existing, ok := rules[normPath]
		if ok {
			// If duplicated in config, preserve MatchChildren=true if either entry enables it
			if app.MatchChildren {
				existing.matchChildren = true
			}
			rules[normPath] = existing
			continue
		}
		rules[normPath] = compiledRule{
			name:          app.Name,
			canonicalPath: normPath,
			matchChildren: app.MatchChildren,
		}
	}

	return &PolicyEngine{
		enabled:       cfg.Enabled,
		mode:          mode,
		killSwitch:    killSwitch,
		ignoreKillSwitchOnDisconnect: cfg.IgnoreKillSwitchOnDisconnect,
		systemDNS:     systemDNS,
		selfPath:      selfNorm,
		antiLoopNames: antiLoopNames,
		rulesByPath:   rules,
		getenv:        getenv,
	}
}

func isAntiLoopPath(normPath, selfNorm string, antiLoopNames map[string]struct{}) bool {
	if normPath == "" {
		return false
	}
	if selfNorm != "" && normPath == selfNorm {
		return true
	}
	base := BaseExeName(normPath)
	_, ok := antiLoopNames[base]
	return ok
}

// IsAntiLoop reports whether the given executable path belongs to PWDTT itself and must always bypass the tunnel.
func (e *PolicyEngine) IsAntiLoop(exePath string) bool {
	norm := NormalizeWindowsPathWithEnv(exePath, e.getenv)
	return isAntiLoopPath(norm, e.selfPath, e.antiLoopNames)
}

// Evaluate determines the routing decision for the given process and its parent chain.
func (e *PolicyEngine) Evaluate(proc ProcessInfo) RouteDecision {
	normExe := NormalizeWindowsPathWithEnv(proc.ExePath, e.getenv)

	// Invariant 1: PWDTT itself (and its direct helper processes) must ALWAYS bypass the tunnel
	if isAntiLoopPath(normExe, e.selfPath, e.antiLoopNames) {
		return DecisionBypassAntiLoop
	}
	for _, parent := range proc.ParentChain {
		normParent := NormalizeWindowsPathWithEnv(parent, e.getenv)
		if isAntiLoopPath(normParent, e.selfPath, e.antiLoopNames) {
			return DecisionBypassAntiLoop
		}
	}

	// Invariant 2: When split tunneling is disabled or set to global, all other apps use VPN
	if !e.enabled || e.mode == "global" {
		return DecisionVPN
	}

	matched := e.matchesAppList(normExe, proc.ParentChain)

	switch e.mode {
	case "include":
		if matched {
			return DecisionVPN
		}
		return DecisionDirect
	case "exclude":
		if matched {
			return DecisionDirect
		}
		return DecisionVPN
	default:
		return DecisionDirect
	}
}

func (e *PolicyEngine) matchesAppList(normExe string, parentChain []string) bool {
	if normExe != "" {
		if _, ok := e.rulesByPath[normExe]; ok {
			return true
		}
		if exeBase := BaseExeName(normExe); exeBase != "" {
			for _, rule := range e.rulesByPath {
				if BaseExeName(rule.canonicalPath) == exeBase {
					return true
				}
			}
		}
	}

	for _, parent := range parentChain {
		normParent := NormalizeWindowsPathWithEnv(parent, e.getenv)
		if normParent == "" {
			continue
		}
		if rule, ok := e.rulesByPath[normParent]; ok && rule.matchChildren {
			return true
		}
		if pBase := BaseExeName(normParent); pBase != "" {
			for _, rule := range e.rulesByPath {
				if rule.matchChildren && BaseExeName(rule.canonicalPath) == pBase {
					return true
				}
			}
		}
	}
	return false
}

// Mode returns the active routing mode ("global", "include", "exclude").
func (e *PolicyEngine) Mode() string {
	return e.mode
}

// Enabled reports whether split tunneling is enabled.
func (e *PolicyEngine) Enabled() bool {
	return e.enabled
}

// KillSwitchFailClosed reports whether fail-closed blocking is active on tunnel outage.
func (e *PolicyEngine) KillSwitchFailClosed() bool {
	return e.killSwitch == "fail_closed"
}

// IgnoreKillSwitchOnDisconnect reports whether manual disconnect (StateDown) bypasses Kill Switch.
func (e *PolicyEngine) IgnoreKillSwitchOnDisconnect() bool {
	return e.ignoreKillSwitchOnDisconnect
}

// SystemDNSPolicy returns "tunnel_dns" or "system_dns".
func (e *PolicyEngine) SystemDNSPolicy() string {
	return e.systemDNS
}

// MatchedAppPaths returns a sorted list of canonical normalized executable paths in the active rule list.
// Used by WFP Kill Switch (T07) to build ALE_APP_ID filter lists.
func (e *PolicyEngine) MatchedAppPaths() []string {
	out := make([]string, 0, len(e.rulesByPath))
	for p := range e.rulesByPath {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
