package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultGitHubAPIBase = "https://api.github.com"

// GitHubClient queries GitHub Releases and Compare APIs with ETag caching (HTTP 304).
type GitHubClient struct {
	apiBaseURL string
	httpClient *http.Client

	mu     sync.Mutex
	etags  map[string]string
	cached map[string][]byte
}

// NewGitHubClient creates a client pointing at api.github.com (or customBaseURL in tests).
func NewGitHubClient(customBaseURL string, httpClient *http.Client) *GitHubClient {
	base := strings.TrimRight(strings.TrimSpace(customBaseURL), "/")
	if base == "" {
		base = defaultGitHubAPIBase
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 12 * time.Second}
	}
	return &GitHubClient{
		apiBaseURL: base,
		httpClient: httpClient,
		etags:      make(map[string]string),
		cached:     make(map[string][]byte),
	}
}

func (c *GitHubClient) getJSON(ctx context.Context, path string) ([]byte, bool, error) {
	url := c.apiBaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PWDTT-SplitTunnel-Updater")

	c.mu.Lock()
	etag := c.etags[path]
	cachedBody := c.cached[path]
	c.mu.Unlock()

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && len(cachedBody) > 0 {
		return append([]byte(nil), cachedBody...), true, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("github API %s returned HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if newETag := resp.Header.Get("ETag"); newETag != "" {
		c.mu.Lock()
		c.etags[path] = newETag
		c.cached[path] = append([]byte(nil), body...)
		c.mu.Unlock()
	}
	return body, false, nil
}

// LatestRelease fetches the latest published release from owner/repo.
func (c *GitHubClient) LatestRelease(ctx context.Context, owner, repo string) (ReleaseInfo, bool, error) {
	path := fmt.Sprintf("/repos/%s/%s/releases/latest", owner, repo)
	body, fromCache, err := c.getJSON(ctx, path)
	if err != nil {
		return ReleaseInfo{}, false, err
	}
	var rel ReleaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return ReleaseInfo{}, fromCache, fmt.Errorf("decode latest release: %w", err)
	}
	return rel, fromCache, nil
}

type githubCompareResp struct {
	AheadBy      int    `json:"ahead_by"`
	TotalCommits int    `json:"total_commits"`
	HTMLURL      string `json:"html_url"`
	Files        []struct {
		Filename string `json:"filename"`
	} `json:"files"`
}

// CompareTags compares baseTag...headTag and detects overlaps with TouchedSeamFiles.
func (c *GitHubClient) CompareTags(ctx context.Context, owner, repo, baseTag, headTag string, seamFiles []string) (CompareSummary, error) {
	path := fmt.Sprintf("/repos/%s/%s/compare/%s...%s", owner, repo, baseTag, headTag)
	body, _, err := c.getJSON(ctx, path)
	if err != nil {
		return CompareSummary{}, err
	}
	var raw githubCompareResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return CompareSummary{}, fmt.Errorf("decode compare response: %w", err)
	}

	seamSet := make(map[string]struct{}, len(seamFiles))
	for _, f := range seamFiles {
		seamSet[strings.ToLower(strings.ReplaceAll(strings.TrimSpace(f), "\\", "/"))] = struct{}{}
	}

	changed := make([]string, 0, len(raw.Files))
	overlap := make([]string, 0)
	for _, f := range raw.Files {
		norm := strings.ReplaceAll(strings.TrimSpace(f.Filename), "\\", "/")
		if norm == "" {
			continue
		}
		changed = append(changed, norm)
		if _, ok := seamSet[strings.ToLower(norm)]; ok {
			overlap = append(overlap, norm)
		}
	}
	sort.Strings(changed)
	sort.Strings(overlap)

	return CompareSummary{
		BaseTag:            baseTag,
		HeadTag:            headTag,
		AheadBy:            raw.AheadBy,
		TotalCommits:       raw.TotalCommits,
		CompareURL:         raw.HTMLURL,
		ChangedFiles:       changed,
		TouchedSeamOverlap: overlap,
	}, nil
}

// IsNewerSemver returns true if latestTag is strictly newer than baseTag (supports vX.Y.Z[-pre]).
func IsNewerSemver(baseTag, latestTag string) bool {
	bMaj, bMin, bPat, bPre, okB := parseSemver(baseTag)
	lMaj, lMin, lPat, lPre, okL := parseSemver(latestTag)
	if !okB || !okL {
		return strings.TrimSpace(latestTag) != "" && strings.TrimSpace(latestTag) != strings.TrimSpace(baseTag)
	}
	if lMaj != bMaj {
		return lMaj > bMaj
	}
	if lMin != bMin {
		return lMin > bMin
	}
	if lPat != bPat {
		return lPat > bPat
	}
	// Stable release is newer than prerelease of same X.Y.Z
	if bPre != "" && lPre == "" {
		return true
	}
	return false
}

func parseSemver(tag string) (maj, min, pat int, pre string, ok bool) {
	s := strings.TrimSpace(tag)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, "", false
	}
	var err error
	if maj, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, "", false
	}
	if min, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, "", false
	}
	if len(parts) == 3 {
		if pat, err = strconv.Atoi(parts[2]); err != nil {
			return 0, 0, 0, "", false
		}
	}
	return maj, min, pat, pre, true
}
