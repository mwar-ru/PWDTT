//go:build windows

package splittunnel

import (
	"os"
	"path/filepath"

	"pwdtt/internal/splittunnel/appmgr"
	"pwdtt/internal/splittunnel/router"
	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/vpn"
)

func newPlatformService() *Service {
	var store *settings.Store
	var initWarn string

	baseDir, err := settings.DefaultBaseDir()
	if err != nil {
		initWarn = err.Error()
	} else {
		store, err = settings.NewStore(baseDir)
		if err != nil {
			initWarn = err.Error()
		}
	}

	resolver := appmgr.NewWindowsProcessResolver()
	inspector := vpn.NewWindowsInspector(vpn.DefaultTunnelInterfaceName)
	adapter := vpn.NewPWDTTAdapter(inspector)

	dllDir, hashes := resolveWinDivertDir()
	tr := router.NewWindowsTrafficRouter(dllDir, hashes, resolver)

	svc := NewServiceWithDeps(store, resolver, adapter, tr, true)
	svc.lastWarn = initWarn
	return svc
}

func resetPlatformWfpRules() error {
	return router.ResetWfpRules()
}

func resolveWinDivertDir() (string, map[string]string) {
	candidates := make([]string, 0, 4)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			exeDir,
			filepath.Join(exeDir, "windivert"),
			filepath.Join(exeDir, "third_party", "windivert"),
		)
	}
	candidates = append(candidates, filepath.Join("third_party", "windivert"))

	selectedDir := "."
	if len(candidates) > 0 {
		selectedDir = candidates[0]
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "WinDivert.dll")); err == nil {
			selectedDir = dir
			break
		}
	}

	var hashes map[string]string
	for _, dir := range append([]string{selectedDir}, candidates...) {
		data, err := os.ReadFile(filepath.Join(dir, "CHECKSUMS.sha256"))
		if err == nil {
			parsed := router.ParseChecksumManifest(string(data))
			if len(parsed) > 0 {
				hashes = parsed
				break
			}
		}
	}
	return selectedDir, hashes
}
