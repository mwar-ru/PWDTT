//go:build !windows

package appmgr

import "os"

type stubProcessBackend struct{}

func (stubProcessBackend) LookupProcess(uint32) (rawProcessInfo, error) {
	return rawProcessInfo{}, ErrProcessNotFound
}

func (stubProcessBackend) ListProcesses() ([]rawProcessInfo, error) {
	return nil, nil
}

// NewWindowsProcessResolver returns a no-op stub resolver on non-Windows platforms
// so cross-platform Linux/macOS PWDTT builds remain green.
func NewWindowsProcessResolver() *ProcessResolver {
	return NewProcessResolverWithBackend(stubProcessBackend{})
}

// SelfExecutablePath returns the current executable path on non-Windows platforms.
func SelfExecutablePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}
