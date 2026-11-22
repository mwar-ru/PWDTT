//go:build !windows

package backend

import "fmt"

func (a *App) InstallUpdate() error {
	return fmt.Errorf("self-update is only supported on Windows")
}

func (a *App) OpenUpdateLog() error {
	return fmt.Errorf("update log is only supported on Windows")
}
