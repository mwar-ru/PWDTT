package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const githubLatestReleaseURL = "https://api.github.com/repos/mwar-ru/mwPWDTT_windows/releases/latest"

type UpdateInfo struct {
	Available    bool   `json:"available"`
	Version      string `json:"version"`
	URL          string `json:"url"`
	Body         string `json:"body"`
	ChecksumsURL string `json:"checksumsUrl"`
	AssetName    string `json:"assetName"`
}

func CheckUpdate(currentVersion string) (*UpdateInfo, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mwPWDTT_windows")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
			BrowserDownloadURL string `json:"browser_download_url"`
			Name               string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	current := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")
	log.Printf("[UPDATE] Release: %s, Current: %s, isNewer: %v", latest, current, isNewer(latest, current))
	if !isNewer(latest, current) {
		return &UpdateInfo{Available: false}, nil
	}

	var downloadURL, checksumsURL, assetName string
	for _, asset := range release.Assets {
		low := strings.ToLower(asset.Name)
		if strings.HasSuffix(low, ".zip") && strings.Contains(low, "windows") && downloadURL == "" {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
		}
		if low == "sha256sums.txt" {
			checksumsURL = asset.BrowserDownloadURL
		}
	}
	if downloadURL == "" {
		downloadURL = release.HTMLURL
	}

	return &UpdateInfo{
		Available:    true,
		Version:      latest,
		URL:          downloadURL,
		Body:         release.Body,
		ChecksumsURL: checksumsURL,
		AssetName:    assetName,
	}, nil
}

func isNewer(a, b string) bool {
	av := parseVersion(a)
	bv := parseVersion(b)
	for i := 0; i < 4; i++ {
		if av[i] > bv[i] {
			return true
		}
		if av[i] < bv[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [4]int {
	var result [4]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	mainPart := v
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		mainPart = v[:idx]
		suf := strings.ToLower(v[idx+1:])
		if strings.HasPrefix(suf, "st.") {
			num := suf[3:]
			if cut := strings.IndexAny(num, "-+."); cut >= 0 {
				num = num[:cut]
			}
			if n, err := strconv.Atoi(num); err == nil {
				result[3] = n
			}
		}
	}
	parts := strings.SplitN(mainPart, ".", 3)
	for i, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			result[i] = n
		}
	}
	return result
}

func parseSHA256ForAsset(content, assetName string) string {
	target := strings.ToLower(strings.TrimSpace(assetName))
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && strings.ToLower(strings.TrimPrefix(fields[1], "*")) == target {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}
