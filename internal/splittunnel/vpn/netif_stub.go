//go:build !windows

package vpn

type stubInterfaceProvider struct{}

func (stubInterfaceProvider) ListInterfaces() ([]InterfaceInfo, error) {
	return nil, nil
}

// NewWindowsInspector returns a no-op Inspector on non-Windows platforms
// so Linux/macOS PWDTT builds remain green.
func NewWindowsInspector(tunnelNameHint string) *Inspector {
	return NewInspector(stubInterfaceProvider{}, tunnelNameHint)
}
