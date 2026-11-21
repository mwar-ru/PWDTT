package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGitHubClientETag304AndCompareOverlap(t *testing.T) {
	var releaseHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/luminescq/PWDTT/releases/latest":
			releaseHits.Add(1)
			if r.Header.Get("If-None-Match") == `"etag-v170"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"etag-v170"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v1.7.0","name":"v1.7.0","html_url":"https://github.com/luminescq/PWDTT/releases/tag/v1.7.0","published_at":"2026-11-21T10:00:00Z","body":"WireGuard improvements"}`))
		case "/repos/luminescq/PWDTT/compare/v1.6.0...v1.7.0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ahead_by":5,"total_commits":5,"html_url":"https://github.com/luminescq/PWDTT/compare/v1.6.0...v1.7.0","files":[{"filename":"backend/wg.go"},{"filename":"README.md"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewGitHubClient(srv.URL, srv.Client())
	svc := NewUpdaterServiceWithClient(client, t.TempDir(), t.TempDir(), UpstreamOwner, UpstreamRepo, BaseReleaseTag, TouchedSeamFiles)

	first := svc.CheckUpdate(context.Background())
	if first.Error != "" || !first.UpdateAvailable || first.LatestTag != "v1.7.0" || first.FromCache {
		t.Fatalf("unexpected first check result: %+v", first)
	}
	if first.Compare == nil || len(first.Compare.TouchedSeamOverlap) != 1 || first.Compare.TouchedSeamOverlap[0] != "backend/wg.go" {
		t.Fatalf("expected backend/wg.go overlap, got %+v", first.Compare)
	}

	second := svc.CheckUpdate(context.Background())
	if !second.FromCache || second.LatestTag != "v1.7.0" {
		t.Fatalf("expected cached 304 response on second check, got %+v", second)
	}
	if releaseHits.Load() != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", releaseHits.Load())
	}

	if !IsNewerSemver("v1.6.0", "v1.6.1") || !IsNewerSemver("v1.6.0", "v2.0.0") || IsNewerSemver("v1.6.0", "v1.6.0") || IsNewerSemver("v1.7.0", "v1.6.9") {
		t.Fatal("IsNewerSemver check failed")
	}
}

func TestWorktreeDryRunCleanAndConflict(t *testing.T) {
	ctx := context.Background()
	repoDir := t.TempDir()
	reportsDir := t.TempDir()

	gitMust := func(args ...string) {
		t.Helper()
		out, err := runGit(ctx, repoDir, args...)
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		p := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	gitMust("init", "-b", "main")
	gitMust("config", "user.name", "Test")
	gitMust("config", "user.email", "test@example.com")

	// Base release v1.6.0
	writeFile("backend/wg.go", "package backend\nconst metric = 1\n")
	writeFile("README.md", "v1.6.0\n")
	gitMust("add", ".")
	gitMust("commit", "-m", "base v1.6.0")
	gitMust("tag", "v1.6.0")

	// Upstream clean branch v1.6.1 (only touches README.md)
	gitMust("checkout", "-b", "upstream-clean", "v1.6.0")
	writeFile("README.md", "v1.6.1\n")
	gitMust("add", ".")
	gitMust("commit", "-m", "upstream v1.6.1")
	gitMust("tag", "v1.6.1")

	// Upstream conflicting branch v1.7.0 (modifies backend/wg.go same line)
	gitMust("checkout", "-b", "upstream-conflict", "v1.6.0")
	writeFile("backend/wg.go", "package backend\nconst metric = 500\n")
	gitMust("add", ".")
	gitMust("commit", "-m", "upstream v1.7.0")
	gitMust("tag", "v1.7.0")

	// Fork branch (modifies backend/wg.go)
	gitMust("checkout", "-b", "feat/windows-split-tunnel", "v1.6.0")
	writeFile("backend/wg.go", "package backend\nconst metric = 9999\n")
	gitMust("add", ".")
	gitMust("commit", "-m", "fork seam change")

	svc := NewUpdaterServiceWithClient(nil, repoDir, reportsDir, UpstreamOwner, UpstreamRepo, BaseReleaseTag, TouchedSeamFiles)

	// 1. Clean merge dry-run
	cleanRes, err := svc.AttemptSync(ctx, "v1.6.1")
	if err != nil {
		t.Fatalf("AttemptSync(v1.6.1) failed: %v", err)
	}
	if !cleanRes.DryRun.Clean {
		t.Fatalf("expected clean merge for v1.6.1, got %+v", cleanRes.DryRun)
	}
	if _, err := ValidateReportPath(reportsDir, cleanRes.ReportPath); err != nil {
		t.Fatalf("invalid clean report path: %v", err)
	}

	// 2. Conflicting merge dry-run
	conflictRes, err := svc.AttemptSync(ctx, "v1.7.0")
	if err != nil {
		t.Fatalf("AttemptSync(v1.7.0) failed: %v", err)
	}
	if conflictRes.DryRun.Clean {
		t.Fatal("expected conflict for v1.7.0")
	}
	if len(conflictRes.DryRun.ConflictFiles) != 1 || conflictRes.DryRun.ConflictFiles[0] != "backend/wg.go" {
		t.Fatalf("expected conflict in backend/wg.go, got %v", conflictRes.DryRun.ConflictFiles)
	}
	reportBytes, err := os.ReadFile(conflictRes.ReportPath)
	if err != nil {
		t.Fatalf("read conflict report: %v", err)
	}
	reportText := string(reportBytes)
	if !strings.Contains(reportText, "CONFLICTS_DETECTED") || !strings.Contains(reportText, "backend/wg.go") {
		t.Fatalf("report missing expected conflict details:\n%s", reportText)
	}

	// 3. Verify main working tree was NEVER dirtied
	statusOut, err := runGit(ctx, repoDir, "status", "--porcelain")
	if err != nil || strings.TrimSpace(statusOut) != "" {
		t.Fatalf("expected clean main working tree after dry-runs, got err=%v status=%q", err, statusOut)
	}
}

func TestValidateReportPathRejectsTraversal(t *testing.T) {
	reportsDir := t.TempDir()
	validPath, err := SaveReport(reportsDir, "v1.7.0", "# Report")
	if err != nil {
		t.Fatalf("SaveReport: %v", err)
	}
	if _, err := ValidateReportPath(reportsDir, validPath); err != nil {
		t.Fatalf("expected valid report path to pass: %v", err)
	}
	if _, err := ValidateReportPath(reportsDir, filepath.Join(reportsDir, "..", "escape.md")); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
	txtPath := filepath.Join(reportsDir, "notes.txt")
	_ = os.WriteFile(txtPath, []byte("x"), 0o600)
	if _, err := ValidateReportPath(reportsDir, txtPath); err == nil {
		t.Fatal("expected non-.md file to be rejected")
	}
}
