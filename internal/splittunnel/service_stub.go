//go:build !windows

package splittunnel

import (
	"pwdtt/internal/splittunnel/router"
	"pwdtt/internal/splittunnel/settings"
)

func newPlatformService() *Service {
	var store *settings.Store
	if baseDir, err := settings.DefaultBaseDir(); err == nil {
		store, _ = settings.NewStore(baseDir)
	}
	return NewServiceWithDeps(store, nil, nil, router.NewWindowsTrafficRouter("", nil, nil), false)
}

func resetPlatformWfpRules() error {
	return nil
}
