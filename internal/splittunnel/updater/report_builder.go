package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildAIReportMarkdown formats an actionable Markdown report for an AI coding agent or developer.
func BuildAIReportMarkdown(check UpdateCheckResult, dryRun DryRunResult) string {
	var b strings.Builder
	status := "CONFLICTS_DETECTED"
	if dryRun.Clean {
		status = "CLEAN_MERGE"
	}

	target := dryRun.TargetRef
	if target == "" {
		target = check.LatestTag
	}
	baseTag := check.BaseTag
	if baseTag == "" {
		baseTag = BaseReleaseTag
	}
	repoURL := check.UpstreamRepoURL
	if repoURL == "" {
		repoURL = UpstreamRepoURL
	}

	b.WriteString("# PWDTT Upstream Sync Report\n\n")
	b.WriteString(fmt.Sprintf("- **Generated:** %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Upstream:** %s\n", repoURL))
	b.WriteString(fmt.Sprintf("- **Base Release:** `%s`\n", baseTag))
	b.WriteString(fmt.Sprintf("- **Target Ref:** `%s`\n", target))
	b.WriteString(fmt.Sprintf("- **Dry-Run Status:** `%s`\n\n", status))

	if check.Release != nil {
		b.WriteString("## Upstream Release Notes\n\n")
		if check.Release.HTMLURL != "" {
			b.WriteString(fmt.Sprintf("- URL: %s\n", check.Release.HTMLURL))
		}
		if check.Release.PublishedAt != "" {
			b.WriteString(fmt.Sprintf("- Published: %s\n", check.Release.PublishedAt))
		}
		if body := strings.TrimSpace(check.Release.Body); body != "" {
			b.WriteString("\n```markdown\n")
			b.WriteString(truncateOutput(body, 4000))
			b.WriteString("\n```\n\n")
		}
	}

	if check.Compare != nil {
		b.WriteString("## Compare API Summary\n\n")
		b.WriteString(fmt.Sprintf("- Commits ahead: %d\n", check.Compare.AheadBy))
		b.WriteString(fmt.Sprintf("- Changed files: %d\n", len(check.Compare.ChangedFiles)))
		if len(check.Compare.TouchedSeamOverlap) > 0 {
			b.WriteString("- **Touched Seam Overlap:**\n")
			for _, f := range check.Compare.TouchedSeamOverlap {
				b.WriteString(fmt.Sprintf("  - `%s`\n", f))
			}
		} else {
			b.WriteString("- **Touched Seam Overlap:** none\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Isolated Git Worktree Dry-Run\n\n")
	if dryRun.Clean {
		b.WriteString("Merge succeeded cleanly with **zero conflicts**.\n\n")
		if len(dryRun.MergedFiles) > 0 {
			b.WriteString("Merged files:\n")
			for _, f := range dryRun.MergedFiles {
				b.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("Conflicts detected in:\n")
		for _, f := range dryRun.ConflictFiles {
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		b.WriteString("\n")
	}

	if dryRun.DiffStat != "" {
		b.WriteString("### Diff Stat\n\n```text\n")
		b.WriteString(dryRun.DiffStat)
		b.WriteString("\n```\n\n")
	}

	b.WriteString("## Instructions for AI Coding Agent\n\n")
	b.WriteString("1. Keep all split-tunneling code strictly isolated inside `internal/splittunnel/`.\n")
	b.WriteString("2. Preserve minimal upstream seams in `backend/wg.go` (`vpn.ShouldSkipDefaultRoute()`, `vpn.NotifyTunnelState(...)`) and `main_windows.go`.\n")
	b.WriteString("3. Resolve only the conflicted files listed above without regressing upstream features.\n")
	b.WriteString("4. Verify after merge:\n")
	b.WriteString("   - `go test -race ./internal/splittunnel/...`\n")
	b.WriteString("   - `GOOS=windows GOARCH=amd64 go test -c -o /dev/null ./internal/splittunnel/...`\n")
	b.WriteString("   - `go test ./tests/...`\n")

	return b.String()
}

// SaveReport writes the Markdown report into reportsDir and returns its absolute path.
func SaveReport(reportsDir, targetTag, markdown string) (string, error) {
	if strings.TrimSpace(reportsDir) == "" {
		return "", errors.New("reports directory is empty")
	}
	absDir, err := filepath.Abs(reportsDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return "", fmt.Errorf("create reports dir: %w", err)
	}

	safeTag := sanitizeFilenamePart(targetTag)
	if safeTag == "" {
		safeTag = "upstream"
	}
	filename := fmt.Sprintf("upstream-sync-%s-%s.md", safeTag, time.Now().UTC().Format("20060102-150405"))
	fullPath := filepath.Join(absDir, filename)

	if err := os.WriteFile(fullPath, []byte(markdown), 0o600); err != nil {
		return "", fmt.Errorf("write report file: %w", err)
	}
	return fullPath, nil
}

// ValidateReportPath ensures reportPath is an existing .md file strictly inside reportsDir.
func ValidateReportPath(reportsDir, reportPath string) (string, error) {
	if strings.TrimSpace(reportsDir) == "" || strings.TrimSpace(reportPath) == "" {
		return "", errors.New("reports directory and report path are required")
	}
	absDir, err := filepath.Abs(filepath.Clean(reportsDir))
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(filepath.Clean(reportPath))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("report path %q escapes reports directory", reportPath)
	}
	if !strings.EqualFold(filepath.Ext(absPath), ".md") {
		return "", errors.New("report path must be a .md file")
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("report path points to a directory")
	}
	return absPath, nil
}

func sanitizeFilenamePart(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "._-")
}
