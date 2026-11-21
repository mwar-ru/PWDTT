package vpn

import (
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultTunnelInterfaceName matches upstream PWDTT's wgIface constant in backend/wg.go.
const DefaultTunnelInterfaceName = "pwdtt-wg"

// TunnelHooks is the minimal seam interface invoked by backend/wg.go on Windows.
type TunnelHooks interface {
	ShouldSkipDefaultRoute() bool
	OnTunnelState(state TunnelState, ifaceName string, err error)
}

var (
	activeHooksMu sync.RWMutex
	activeHooks   TunnelHooks
)

// SetActiveHooks registers (or clears with nil) the global hooks consumed by backend/wg.go.
func SetActiveHooks(h TunnelHooks) {
	activeHooksMu.Lock()
	activeHooks = h
	activeHooksMu.Unlock()
}

// ShouldSkipDefaultRoute is called by backend/wg.go during Windows tunnel setup.
// Returns false if split tunneling is not registered or not in "include" mode,
// preserving 100% original upstream PWDTT behavior by default.
func ShouldSkipDefaultRoute() bool {
	activeHooksMu.RLock()
	h := activeHooks
	activeHooksMu.RUnlock()
	if h == nil {
		return false
	}
	return h.ShouldSkipDefaultRoute()
}

// NotifyTunnelState is called by backend/wg.go on Windows tunnel lifecycle transitions.
func NotifyTunnelState(state TunnelState, ifaceName string, err error) {
	activeHooksMu.RLock()
	h := activeHooks
	activeHooksMu.RUnlock()
	if h == nil {
		return
	}
	h.OnTunnelState(state, ifaceName, err)
}

// IsDefaultRouteCIDR reports whether a WireGuard AllowedIPs CIDR hijacks the OS default route.
func IsDefaultRouteCIDR(cidr string) bool {
	trimmed := strings.TrimSpace(cidr)
	switch trimmed {
	case "0.0.0.0/0", "0.0.0.0/1", "128.0.0.0/1", "::/0", "::/1", "8000::/1":
		return true
	}
	prefix, err := netip.ParsePrefix(trimmed)
	if err != nil {
		return false
	}
	return prefix.Bits() <= 1
}

type subscriberEntry struct {
	id uint64
	fn func(TunnelEvent)
}

// PWDTTAdapter bridges upstream backend/wg.go lifecycle hooks with Split Tunneling components.
type PWDTTAdapter struct {
	inspector        *Inspector
	skipDefaultRoute atomic.Bool
	inspectRetryWait time.Duration

	mu          sync.RWMutex
	state       TunnelState
	lastTopo    *NetworkTopology
	nextSubID   uint64
	subscribers []subscriberEntry
}

// NewPWDTTAdapter creates a new adapter using the provided network Inspector.
// If inspector is nil, a Windows inspector targeting DefaultTunnelInterfaceName is used.
func NewPWDTTAdapter(inspector *Inspector) *PWDTTAdapter {
	if inspector == nil {
		inspector = NewWindowsInspector(DefaultTunnelInterfaceName)
	}
	return &PWDTTAdapter{
		inspector:        inspector,
		inspectRetryWait: 100 * time.Millisecond,
		state:            StateDown,
	}
}

// SetSplitTunnelMode updates whether the Windows Wintun interface should skip taking the OS default route.
// Only enabled + "include" mode skips default route hijacking so unselected apps stay direct natively.
func (a *PWDTTAdapter) SetSplitTunnelMode(enabled bool, mode string) {
	m := strings.ToLower(strings.TrimSpace(mode))
	a.skipDefaultRoute.Store(enabled && m == "include")
}

// ShouldSkipDefaultRoute implements TunnelHooks.
func (a *PWDTTAdapter) ShouldSkipDefaultRoute() bool {
	return a.skipDefaultRoute.Load()
}

// State returns the last known tunnel lifecycle state.
func (a *PWDTTAdapter) State() TunnelState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// LastTopology returns a copy of the most recently inspected NetworkTopology, if available.
func (a *PWDTTAdapter) LastTopology() *NetworkTopology {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.lastTopo == nil {
		return nil
	}
	cp := *a.lastTopo
	return &cp
}

// Subscribe registers a listener for TunnelEvent notifications and returns an unsubscribe function.
func (a *PWDTTAdapter) Subscribe(fn func(TunnelEvent)) func() {
	if fn == nil {
		return func() {}
	}
	a.mu.Lock()
	a.nextSubID++
	id := a.nextSubID
	a.subscribers = append(a.subscribers, subscriberEntry{id: id, fn: fn})
	a.mu.Unlock()

	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for i, sub := range a.subscribers {
			if sub.id == id {
				a.subscribers = append(a.subscribers[:i], a.subscribers[i+1:]...)
				break
			}
		}
	}
}

// OnTunnelState implements TunnelHooks.
func (a *PWDTTAdapter) OnTunnelState(state TunnelState, ifaceName string, hookErr error) {
	var topo *NetworkTopology
	if state == StateUp && a.inspector != nil {
		if t, err := a.inspectUntilTunnelReady(); err == nil {
			topo = &t
		}
	}

	a.mu.Lock()
	a.state = state
	if topo != nil {
		cp := *topo
		a.lastTopo = &cp
	} else if state == StateDown {
		a.lastTopo = nil
	}
	subs := append([]subscriberEntry(nil), a.subscribers...)
	a.mu.Unlock()

	ev := TunnelEvent{
		State:    state,
		Topology: topo,
		Err:      hookErr,
	}
	for _, sub := range subs {
		sub.fn(ev)
	}
}

func (a *PWDTTAdapter) inspectUntilTunnelReady() (NetworkTopology, error) {
	var lastTopo NetworkTopology
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		topo, err := a.inspector.Inspect()
		if err == nil {
			lastTopo = topo
			if topo.Tunnel != nil && topo.Tunnel.IPv4.IsValid() {
				return topo, nil
			}
		} else {
			lastErr = err
		}
		if attempt < 2 && a.inspectRetryWait > 0 {
			time.Sleep(a.inspectRetryWait)
		}
	}
	return lastTopo, lastErr
}
