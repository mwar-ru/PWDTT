package updater

import "time"

const (
	UpstreamOwner   = "luminescq"
	UpstreamRepo    = "PWDTT"
	BaseReleaseTag  = "v1.6.0"
	UpstreamRepoURL = "https://github.com/luminescq/PWDTT"
)

// TouchedSeamFiles lists upstream files modified by this fork so Compare API
// immediately flags potential merge conflicts before a dry-run merge.
var TouchedSeamFiles = []string{
	"backend/wg.go",
	"main_windows.go",
	"frontend/src/App.tsx",
	"frontend/src/components/Sidebar.tsx",
}

// ReleaseInfo describes a GitHub Release from upstream.
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
}

// CompareSummary summarizes changed files between BaseReleaseTag and a newer upstream tag.
type CompareSummary struct {
	BaseTag            string   `json:"base_tag"`
	HeadTag            string   `json:"head_tag"`
	AheadBy            int      `json:"ahead_by"`
	TotalCommits       int      `json:"total_commits"`
	CompareURL         string   `json:"compare_url"`
	ChangedFiles       []string `json:"changed_files"`
	TouchedSeamOverlap []string `json:"touched_seam_overlap"`
}

// UpdateCheckResult is returned to the UI when checking for upstream PWDTT releases.
type UpdateCheckResult struct {
	UpstreamRepoURL string          `json:"upstream_repo_url"`
	BaseTag         string          `json:"base_tag"`
	LatestTag       string          `json:"latest_tag"`
	UpdateAvailable bool            `json:"update_available"`
	FromCache       bool            `json:"from_cache"`
	CheckedAt       time.Time       `json:"checked_at"`
	Release         *ReleaseInfo    `json:"release,omitempty"`
	Compare         *CompareSummary `json:"compare,omitempty"`
	Error           string          `json:"error,omitempty"`
}

// DryRunResult captures the outcome of a non-destructive merge inside a temporary git worktree.
type DryRunResult struct {
	TargetRef     string   `json:"target_ref"`
	Clean         bool     `json:"clean"`
	MergedFiles   []string `json:"merged_files,omitempty"`
	ConflictFiles []string `json:"conflict_files,omitempty"`
	DiffStat      string   `json:"diff_stat,omitempty"`
	RawOutput     string   `json:"raw_output,omitempty"`
}

// SyncAttemptResult combines dry-run merge status with the generated AI-agent Markdown report.
type SyncAttemptResult struct {
	TargetRef     string       `json:"target_ref"`
	DryRun        DryRunResult `json:"dry_run"`
	ReportPath    string       `json:"report_path"`
	ReportPreview string       `json:"report_preview"`
	Summary       string       `json:"summary"`
}
