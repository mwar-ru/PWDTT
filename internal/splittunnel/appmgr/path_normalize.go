package appmgr

import (
	"os"
	"regexp"
	"strings"
)

var winEnvVarRe = regexp.MustCompile(`%([^%]+)%`)

// NormalizeWindowsPath converts a Windows executable path into a canonical lowercase form
// suitable for deterministic matching across WinAPI, WinDivert, WFP, and JSON configs.
func NormalizeWindowsPath(raw string) string {
	return NormalizeWindowsPathWithEnv(raw, os.Getenv)
}

// NormalizeWindowsPathWithEnv normalizes a Windows path using a custom env lookup function.
func NormalizeWindowsPathWithEnv(raw string, getenv func(string) string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}

	// Strip surrounding quotes if present (e.g. from command lines or user input)
	p = strings.Trim(p, `"`)
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	// Expand %ENV% placeholders (case-insensitive fallback handled by caller or OS)
	if getenv != nil && strings.Contains(p, "%") {
		p = winEnvVarRe.ReplaceAllStringFunc(p, func(match string) string {
			name := match[1 : len(match)-1]
			if val := getenv(name); val != "" {
				return val
			}
			return match
		})
	}

	// Normalize separators to Windows backslashes
	p = strings.ReplaceAll(p, "/", `\`)

	// Strip Win32 / NT extended path prefixes
	lower := strings.ToLower(p)
	switch {
	case strings.HasPrefix(lower, `\\?\unc\`):
		p = `\\` + p[len(`\\?\unc\`):]
	case strings.HasPrefix(lower, `\??\unc\`):
		p = `\\` + p[len(`\??\unc\`):]
	case strings.HasPrefix(lower, `\\?\`):
		p = p[len(`\\?\`):]
	case strings.HasPrefix(lower, `\??\`):
		p = p[len(`\??\`):]
	}

	// Preserve UNC prefix (\\server\share) or NT device prefix (\Device\...)
	prefix := ""
	rest := p
	if strings.HasPrefix(p, `\\`) {
		prefix = `\\`
		rest = strings.TrimLeft(p, `\`)
	} else if strings.HasPrefix(p, `\`) {
		prefix = `\`
		rest = strings.TrimLeft(p, `\`)
	}

	// Split and resolve "." and ".." deterministically regardless of host OS (Linux/Windows)
	rawParts := strings.Split(rest, `\`)
	parts := make([]string, 0, len(rawParts))
	for _, seg := range rawParts {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			// Do not pop drive root like "C:" or UNC server name
			minKeep := 0
			if len(parts) > 0 && len(parts[0]) == 2 && parts[0][1] == ':' {
				minKeep = 1
			} else if prefix == `\\` && len(parts) > 0 {
				minKeep = 1
			}
			if len(parts) > minKeep {
				parts = parts[:len(parts)-1]
			}
			continue
		}
		parts = append(parts, seg)
	}

	if len(parts) == 0 {
		return strings.ToLower(prefix)
	}

	return strings.ToLower(prefix + strings.Join(parts, `\`))
}

// NormalizeNTDevicePath translates an NT device path (e.g. \Device\HarddiskVolume3\Program Files\...)
// into a standard DOS drive path using driveMap (key: lowercase device prefix, value: "c:").
func NormalizeNTDevicePath(raw string, driveMap map[string]string) string {
	norm := NormalizeWindowsPath(raw)
	if norm == "" || len(driveMap) == 0 {
		return norm
	}

	// Match longest device prefix first to avoid partial collisions (e.g. Volume1 vs Volume10)
	bestDev := ""
	bestDrive := ""
	for dev, drive := range driveMap {
		normDev := strings.TrimRight(NormalizeWindowsPath(dev), `\`)
		if normDev == "" {
			continue
		}
		if norm == normDev || strings.HasPrefix(norm, normDev+`\`) {
			if len(normDev) > len(bestDev) {
				bestDev = normDev
				bestDrive = strings.ToLower(strings.TrimRight(strings.TrimSpace(drive), `\`))
			}
		}
	}

	if bestDev == "" || bestDrive == "" {
		return norm
	}
	return bestDrive + norm[len(bestDev):]
}

// BaseExeName returns the lowercase executable filename (e.g. "firefox.exe") from a Windows path.
func BaseExeName(path string) string {
	norm := NormalizeWindowsPath(path)
	if norm == "" {
		return ""
	}
	idx := strings.LastIndexByte(norm, '\\')
	if idx >= 0 {
		return norm[idx+1:]
	}
	return norm
}
