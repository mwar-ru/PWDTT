package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// RunDryRunMerge performs a completely non-destructive merge test of targetRef
// inside an isolated temporary git worktree, leaving the main working tree untouched.
func RunDryRunMerge(ctx context.Context, repoDir, targetRef string) (DryRunResult, error) {
	repoDir = strings.TrimSpace(repoDir)
	targetRef = strings.TrimSpace(targetRef)
	if repoDir == "" {
		return DryRunResult{}, errors.New("repository directory is empty")
	}
	if targetRef == "" {
		return DryRunResult{}, errors.New("target git ref is empty")
	}
	// Reject option-like refs starting with '-' to prevent git flag injection.
	if strings.HasPrefix(targetRef, "-") {
		return DryRunResult{}, fmt.Errorf("invalid git ref %q", targetRef)
	}

	if _, err := runGit(ctx, repoDir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return DryRunResult{}, fmt.Errorf("not a git repository: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "pwdtt-upstream-dryrun-*")
	if err != nil {
		return DryRunResult{}, fmt.Errorf("create temp worktree dir: %w", err)
	}
	// git worktree add requires a non-existent or empty target; remove empty dir so git creates it cleanly.
	_ = os.Remove(tmpDir)

	if out, err := runGit(ctx, repoDir, "worktree", "add", "--detach", tmpDir, "HEAD"); err != nil {
		return DryRunResult{}, fmt.Errorf("git worktree add failed: %w (%s)", err, strings.TrimSpace(out))
	}
	defer func() {
		_, _ = runGit(context.Background(), repoDir, "worktree", "remove", "--force", tmpDir)
		_ = os.RemoveAll(tmpDir)
	}()

	res := DryRunResult{TargetRef: targetRef}
	mergeOut, mergeErr := runGit(ctx, tmpDir, "merge", "--no-commit", "--no-ff", targetRef)
	res.RawOutput = truncateOutput(strings.TrimSpace(mergeOut), 8192)

	if mergeErr == nil {
		res.Clean = true
		if filesOut, err := runGit(ctx, tmpDir, "diff", "--cached", "--name-only"); err == nil {
			res.MergedFiles = splitNonEmptyLines(filesOut)
		}
		if statOut, err := runGit(ctx, tmpDir, "diff", "--cached", "--stat"); err == nil {
			res.DiffStat = strings.TrimSpace(statOut)
		}
		_, _ = runGit(ctx, tmpDir, "merge", "--abort")
		return res, nil
	}

	// Merge returned non-zero: check if there are unmerged conflict files vs invalid ref.
	conflictsOut, confErr := runGit(ctx, tmpDir, "diff", "--name-only", "--diff-filter=U")
	conflicts := splitNonEmptyLines(conflictsOut)
	if confErr == nil && len(conflicts) > 0 {
		res.Clean = false
		res.ConflictFiles = conflicts
		if statOut, err := runGit(ctx, tmpDir, "diff", "--stat"); err == nil {
			res.DiffStat = strings.TrimSpace(statOut)
		}
		_, _ = runGit(ctx, tmpDir, "merge", "--abort")
		return res, nil
	}

	_, _ = runGit(ctx, tmpDir, "merge", "--abort")
	return res, fmt.Errorf("git merge dry-run failed for %q: %w (%s)", targetRef, mergeErr, res.RawOutput)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func splitNonEmptyLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t != "" {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

func truncateOutput(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "\n... (truncated)"
}
