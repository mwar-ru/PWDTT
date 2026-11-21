package updater

import (
	"context"
	"fmt"
	"time"
)

// UpdaterService coordinates GitHub release checks, isolated worktree dry-runs, and AI reports.
type UpdaterService struct {
	client     *GitHubClient
	repoDir    string
	reportsDir string
	owner      string
	repo       string
	baseTag    string
	seamFiles  []string
}

// NewUpdaterService builds the default updater service targeting luminescq/PWDTT.
func NewUpdaterService(repoDir, reportsDir string) *UpdaterService {
	return NewUpdaterServiceWithClient(
		NewGitHubClient("", nil),
		repoDir,
		reportsDir,
		UpstreamOwner,
		UpstreamRepo,
		BaseReleaseTag,
		TouchedSeamFiles,
	)
}

// NewUpdaterServiceWithClient builds an UpdaterService with custom client/repo settings (useful for tests).
func NewUpdaterServiceWithClient(
	client *GitHubClient,
	repoDir, reportsDir, owner, repo, baseTag string,
	seamFiles []string,
) *UpdaterService {
	if client == nil {
		client = NewGitHubClient("", nil)
	}
	return &UpdaterService{
		client:     client,
		repoDir:    repoDir,
		reportsDir: reportsDir,
		owner:      owner,
		repo:       repo,
		baseTag:    baseTag,
		seamFiles:  append([]string(nil), seamFiles...),
	}
}

// CheckUpdate queries upstream GitHub Releases and Compare API (ETag cached).
func (s *UpdaterService) CheckUpdate(ctx context.Context) UpdateCheckResult {
	res := UpdateCheckResult{
		UpstreamRepoURL: fmt.Sprintf("https://github.com/%s/%s", s.owner, s.repo),
		BaseTag:         s.baseTag,
		CheckedAt:       time.Now().UTC(),
	}

	rel, fromCache, err := s.client.LatestRelease(ctx, s.owner, s.repo)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Release = &rel
	res.LatestTag = rel.TagName
	res.FromCache = fromCache
	res.UpdateAvailable = IsNewerSemver(s.baseTag, rel.TagName)

	if res.UpdateAvailable {
		if cmp, cmpErr := s.client.CompareTags(ctx, s.owner, s.repo, s.baseTag, rel.TagName, s.seamFiles); cmpErr == nil {
			res.Compare = &cmp
		}
	}
	return res
}

// AttemptSync runs a non-destructive dry-run merge in a temp worktree and writes a Markdown AI report.
func (s *UpdaterService) AttemptSync(ctx context.Context, targetRef string) (SyncAttemptResult, error) {
	check := UpdateCheckResult{
		UpstreamRepoURL: fmt.Sprintf("https://github.com/%s/%s", s.owner, s.repo),
		BaseTag:         s.baseTag,
		LatestTag:       targetRef,
		CheckedAt:       time.Now().UTC(),
	}

	dryRun, err := RunDryRunMerge(ctx, s.repoDir, targetRef)
	if err != nil {
		return SyncAttemptResult{}, err
	}

	markdown := BuildAIReportMarkdown(check, dryRun)
	reportPath, err := SaveReport(s.reportsDir, targetRef, markdown)
	if err != nil {
		return SyncAttemptResult{}, err
	}

	summary := fmt.Sprintf("Clean dry-run merge for %s (%d files)", targetRef, len(dryRun.MergedFiles))
	if !dryRun.Clean {
		summary = fmt.Sprintf("Conflicts detected for %s in %d file(s)", targetRef, len(dryRun.ConflictFiles))
	}

	return SyncAttemptResult{
		TargetRef:     targetRef,
		DryRun:        dryRun,
		ReportPath:    reportPath,
		ReportPreview: truncateOutput(markdown, 2000),
		Summary:       summary,
	}, nil
}

// OpenReport opens a previously generated report file inside reportsDir.
func (s *UpdaterService) OpenReport(reportPath string) error {
	return OpenReportInEditor(s.reportsDir, reportPath)
}
