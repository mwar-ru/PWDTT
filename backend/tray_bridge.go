package backend

import "sync"

var (
	trayStateMu      sync.Mutex
	trayStateHandler func(TrayState)
	lastTrayState    *TrayState
)

// SetTrayStateHandler registers a callback (used by main_windows.go) that updates
// the native system tray menu whenever connection or routing mode state changes.
func SetTrayStateHandler(fn func(TrayState)) {
	trayStateMu.Lock()
	trayStateHandler = fn
	var pending *TrayState
	if lastTrayState != nil {
		cp := *lastTrayState
		pending = &cp
	}
	trayStateMu.Unlock()

	if fn != nil && pending != nil {
		fn(*pending)
	}
}

// UpdateTrayState is called from the Wails frontend whenever tunnel or mode state changes.
func (a *App) UpdateTrayState(st TrayState) {
	trayStateMu.Lock()
	cp := st
	lastTrayState = &cp
	fn := trayStateHandler
	trayStateMu.Unlock()

	if fn != nil {
		fn(st)
	}
}

// GetStartMinimized returns whether the app should start hidden in the system tray.
func (a *App) GetStartMinimized() bool {
	return a.store.LoadSettings().StartMinimized
}

// SetStartMinimized persists the start-minimized-to-tray preference in config.json.
func (a *App) SetStartMinimized(v bool) error {
	settings := a.store.LoadSettings()
	settings.StartMinimized = v
	return a.store.SaveSettings(settings)
}

// ShouldStartHidden is read by main_windows.go before wails.Run creates the window.
func (a *App) ShouldStartHidden() bool {
	return a.store.LoadSettings().StartMinimized
}
