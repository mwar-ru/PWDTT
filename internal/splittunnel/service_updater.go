package splittunnel

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"pwdtt/internal/splittunnel/settings"
	"pwdtt/internal/splittunnel/updater"
)

var (
	updaterOnce sync.Once
	updaterInst *updater.UpdaterService
)

func getUpdaterService() *updater.UpdaterService {
	updaterOnce.Do(func() {
		repoDir, _ := os.Getwd()
		reportsDir := filepath.Join(os.TempDir(), "pwdtt-reports")
		if baseDir, err := settings.DefaultBaseDir(); err == nil && baseDir != "" {
			reportsDir = filepath.Join(baseDir, "reports")
		}
		updaterInst = updater.NewUpdaterService(repoDir, reportsDir)
	})
	return updaterInst
}

// CheckUpstreamUpdate checks luminescq/PWDTT for newer releases relative to v1.6.0.
func (s *Service) CheckUpstreamUpdate() updater.UpdateCheckResult {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	return getUpdaterService().CheckUpdate(ctx)
}

// AttemptUpstreamSync runs an isolated worktree dry-run merge and generates a Markdown AI report.
func (s *Service) AttemptUpstreamSync(targetRef string) (updater.SyncAttemptResult, error) {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	return getUpdaterService().AttemptSync(ctx, targetRef)
}

// OpenUpstreamReport validates and opens the generated AI sync report in the default editor.
func (s *Service) OpenUpstreamReport(reportPath string) error {
	return getUpdaterService().OpenReport(reportPath)
}
