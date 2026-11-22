//go:build windows

package backend

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"
)

// InstallUpdate downloads the latest Windows release zip, verifies SHA256SUMS.txt,
// swaps mwPWDTT_windows.exe after graceful shutdown, and relaunches the app.
func (a *App) InstallUpdate() error {
	info, err := CheckUpdate(Version)
	if err != nil {
		return fmt.Errorf("check release: %w", err)
	}
	if !info.Available {
		return fmt.Errorf("already on latest version v%s", Version)
	}
	if info.URL == "" || !strings.HasSuffix(strings.ToLower(info.AssetName), ".zip") {
		return fmt.Errorf("release v%s has no Windows .zip asset", info.Version)
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	appDir := filepath.Dir(exePath)

	tmpDir, err := os.MkdirTemp("", "mwpwdtt-update-*")
	if err != nil {
		return err
	}
	zipPath := filepath.Join(tmpDir, "release.zip")
	client := &http.Client{Timeout: 120 * time.Second}

	if err := downloadToFile(client, info.URL, zipPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("download update: %w", err)
	}

	if info.ChecksumsURL != "" {
		sumsPath := filepath.Join(tmpDir, "SHA256SUMS.txt")
		if err := downloadToFile(client, info.ChecksumsURL, sumsPath); err != nil {
			_ = os.RemoveAll(tmpDir)
			return fmt.Errorf("download SHA256SUMS.txt: %w", err)
		}
		sumsRaw, err := os.ReadFile(sumsPath)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return err
		}
		if expected := parseSHA256ForAsset(string(sumsRaw), info.AssetName); expected != "" {
			actual, err := computeFileSHA256(zipPath)
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				return err
			}
			if actual != expected {
				_ = os.RemoveAll(tmpDir)
				return fmt.Errorf("SHA-256 mismatch for %s", info.AssetName)
			}
		}
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := unzipArchiveSafe(zipPath, extractDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("extract zip: %w", err)
	}

	newExe := findFileByName(extractDir, filepath.Base(exePath))
	if newExe == "" {
		newExe = findFileByName(extractDir, "mwPWDTT_windows.exe")
	}
	if newExe == "" {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("mwPWDTT_windows.exe not found in archive")
	}
	stageDir := filepath.Dir(newExe)

	a.Disconnect()

	ps := fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
$p = %d
$src = '%s'
$dst = '%s'
$exe = '%s'
$tmp = '%s'
for ($i = 0; $i -lt 60; $i++) {
    if (-not (Get-Process -Id $p -ErrorAction SilentlyContinue)) { break }
    Start-Sleep -Milliseconds 300
}
Start-Sleep -Milliseconds 400
Get-ChildItem -LiteralPath $src -File | ForEach-Object {
    $to = Join-Path $dst $_.Name
    if (($_.Name -ieq 'WinDivert64.sys' -or $_.Name -ieq 'WinDivert.dll') -and (Test-Path -LiteralPath $to)) {
        $h1 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
        $h2 = (Get-FileHash -LiteralPath $to -Algorithm SHA256).Hash
        if ($h1 -eq $h2) { return }
    }
    for ($t = 0; $t -lt 15; $t++) {
        try { Copy-Item -LiteralPath $_.FullName -Destination $to -Force -ErrorAction Stop; break }
        catch { Start-Sleep -Milliseconds 350 }
    }
}
Start-Process -FilePath $exe -WorkingDirectory $dst
Start-Sleep -Seconds 2
Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
`, os.Getpid(), escapePS(stageDir), escapePS(appDir), escapePS(filepath.Join(appDir, filepath.Base(newExe))), escapePS(tmpDir))

	scriptPath := filepath.Join(tmpDir, "apply_update.ps1")
	if err := os.WriteFile(scriptPath, []byte(ps), 0600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000 | 0x00000008,
	}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return err
	}

	go func() {
		time.Sleep(250 * time.Millisecond)
		if a.ctx != nil {
			wails.Quit(a.ctx)
		}
		time.Sleep(1500 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func escapePS(s string) string { return strings.ReplaceAll(s, "'", "''") }

func downloadToFile(client *http.Client, url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mwPWDTT_windows")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func unzipArchiveSafe(zipPath, dstDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	cleanDst := filepath.Clean(dstDir) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(dstDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDst) && filepath.Clean(target) != filepath.Clean(dstDir) {
			return fmt.Errorf("unsafe zip entry: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, cerr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if cerr != nil {
			return cerr
		}
	}
	return nil
}

func findFileByName(root, name string) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(d.Name(), name) {
			found = p
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
