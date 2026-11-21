package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var defaultEnvVars = []string{
	"ProgramFiles(x86)",
	"ProgramFiles",
	"LocalAppData",
	"AppData",
	"UserProfile",
	"SystemRoot",
	"WinDir",
}

var envVarRegex = regexp.MustCompile(`%([^%]+)%`)

// AppConfig represents configuration for a single targeted application.
type AppConfig struct {
	Name          string `json:"name"`
	ExePath       string `json:"exe_path"`
	MatchChildren bool   `json:"match_children"`
}

// Config represents the split tunneling configuration schema v1.
type Config struct {
	SchemaVersion    int         `json:"schema_version"`
	Enabled          bool        `json:"enabled"`
	Mode             string      `json:"mode"`
	KillSwitch       string      `json:"kill_switch"`
	WFPCrashMode     string      `json:"wfp_crash_mode"`
	SystemDNS        string      `json:"system_dns"`
	PortableEnvPaths bool        `json:"portable_env_paths"`
	Apps             []AppConfig `json:"apps"`
}

// BootstrapConfig holds the pointer to the active split_tunnel.json.
type BootstrapConfig struct {
	ConfigPath string `json:"config_path"`
}

// DefaultConfig returns standard initial configuration (Schema v1).
func DefaultConfig() Config {
	return Config{
		SchemaVersion:    1,
		Enabled:          true,
		Mode:             "include",
		KillSwitch:       "fail_closed",
		WFPCrashMode:     "dynamic",
		SystemDNS:        "tunnel_dns",
		PortableEnvPaths: true,
		Apps:             make([]AppConfig, 0),
	}
}

func cloneConfig(cfg Config) Config {
	cp := cfg
	if cfg.Apps != nil {
		cp.Apps = append([]AppConfig(nil), cfg.Apps...)
	} else {
		cp.Apps = make([]AppConfig, 0)
	}
	return cp
}

func normalizeConfig(cfg Config) Config {
	res := cloneConfig(cfg)
	if res.SchemaVersion <= 0 {
		res.SchemaVersion = 1
	}
	switch res.Mode {
	case "global", "include", "exclude":
	default:
		res.Mode = "include"
	}
	switch res.KillSwitch {
	case "fail_closed", "fail_open":
	default:
		res.KillSwitch = "fail_closed"
	}
	switch res.WFPCrashMode {
	case "dynamic", "persistent_guard":
	default:
		res.WFPCrashMode = "dynamic"
	}
	switch res.SystemDNS {
	case "tunnel_dns", "system_dns":
	default:
		res.SystemDNS = "tunnel_dns"
	}
	return res
}

// Store manages loading, saving, and path resolution for split_tunnel.json settings.
type Store struct {
	mu                sync.RWMutex
	baseDir           string
	bootstrapPath     string
	lastKnownGoodPath string
	configPath        string
	config            Config
}

// DefaultBaseDir returns the default application data directory (%AppData%/PWDTT).
func DefaultBaseDir() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user config dir: %w", err)
	}
	return filepath.Join(userConfigDir, "PWDTT"), nil
}

// NewStore initializes a new Store instance.
// If customBaseDir is empty, DefaultBaseDir is used.
func NewStore(customBaseDir string) (*Store, error) {
	baseDir := customBaseDir
	if baseDir == "" {
		dir, err := DefaultBaseDir()
		if err != nil {
			return nil, err
		}
		baseDir = dir
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory %s: %w", baseDir, err)
	}

	store := &Store{
		baseDir:           baseDir,
		bootstrapPath:     filepath.Join(baseDir, "split_tunnel_bootstrap.json"),
		lastKnownGoodPath: filepath.Join(baseDir, "split_tunnel_last_known_good.json"),
		configPath:        filepath.Join(baseDir, "split_tunnel.json"),
		config:            DefaultConfig(),
	}

	return store, nil
}

// Load loads settings from bootstrap target file, fallback last-known-good, or defaults.
func (s *Store) Load() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Determine target config path from bootstrap pointer
	if _, err := os.Stat(s.bootstrapPath); err == nil {
		data, readErr := os.ReadFile(s.bootstrapPath)
		if readErr == nil {
			var boot BootstrapConfig
			if jsonErr := json.Unmarshal(data, &boot); jsonErr == nil && strings.TrimSpace(boot.ConfigPath) != "" {
				s.configPath = strings.TrimSpace(boot.ConfigPath)
			}
		}
	}

	// 2. Try reading active configPath
	cfg, err := s.readConfigFile(s.configPath)
	if err == nil {
		s.config = *cfg
		// Refresh local LKG cache whenever primary cloud/local config is successfully loaded
		_ = s.saveToFileLocked(s.lastKnownGoodPath, &s.config)
		cp := cloneConfig(s.config)
		return &cp, nil
	}

	// 3. Fallback to last-known-good cache if primary read fails
	if s.configPath != s.lastKnownGoodPath {
		if lkgCfg, lkgErr := s.readConfigFile(s.lastKnownGoodPath); lkgErr == nil {
			s.config = *lkgCfg
			_ = s.saveToFileLocked(s.configPath, &s.config)
			cp := cloneConfig(s.config)
			return &cp, nil
		}
	}

	// 4. Fallback to default configuration
	defCfg := DefaultConfig()
	s.config = defCfg
	_ = s.saveToFileLocked(s.configPath, &s.config)
	_ = s.saveToFileLocked(s.lastKnownGoodPath, &s.config)
	s.saveBootstrapLocked()

	cp := cloneConfig(s.config)
	return &cp, nil
}

// Save writes config to active configPath atomically, updates LKG backup and in-memory cache.
func (s *Store) Save(cfg *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg == nil {
		return errors.New("cannot save nil config")
	}

	norm := normalizeConfig(*cfg)

	// Save to active configPath
	if err := s.saveToFileLocked(s.configPath, &norm); err != nil {
		return fmt.Errorf("failed to save config to %s: %w", s.configPath, err)
	}

	// Save backup to last-known-good
	_ = s.saveToFileLocked(s.lastKnownGoodPath, &norm)

	// Save bootstrap pointer
	s.saveBootstrapLocked()

	// Update in-memory config with expanded paths
	s.config = s.expandConfigPaths(norm)
	return nil
}

// GetConfig returns a copy of current in-memory config.
func (s *Store) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.config)
}

// GetConfigPath returns active config path.
func (s *Store) GetConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configPath
}

// SetConfigPath updates active split_tunnel.json path (e.g., OneDrive folder).
// If moveExisting is true and current config file exists, it is moved to newPath.
// If moveExisting is false and a valid config already exists at newPath (e.g., synced from another PC via OneDrive),
// it is loaded instead of overwritten.
func (s *Store) SetConfigPath(newPath string, moveExisting bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	trimmed := strings.TrimSpace(newPath)
	cleaned := filepath.Clean(trimmed)
	if cleaned == "" || cleaned == "." {
		return errors.New("invalid target config path")
	}

	// If directory, append standard filename
	if fi, err := os.Stat(cleaned); err == nil && fi.IsDir() {
		cleaned = filepath.Join(cleaned, "split_tunnel.json")
	} else if strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, "\\") {
		cleaned = filepath.Join(cleaned, "split_tunnel.json")
	}

	if cleaned == s.configPath {
		return nil
	}

	oldPath := s.configPath
	targetDir := filepath.Dir(cleaned)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	if moveExisting {
		if data, err := os.ReadFile(oldPath); err == nil {
			if writeErr := atomicWriteFile(cleaned, data, 0644); writeErr != nil {
				return fmt.Errorf("failed to move config to %s: %w", cleaned, writeErr)
			}
			_ = os.Remove(oldPath)
		} else {
			if saveErr := s.saveToFileLocked(cleaned, &s.config); saveErr != nil {
				return fmt.Errorf("failed to write config to new path %s: %w", cleaned, saveErr)
			}
		}
	} else {
		if existingCfg, err := s.readConfigFile(cleaned); err == nil {
			// Preserve existing synced config (e.g., OneDrive config from another PC)
			s.config = *existingCfg
			_ = s.saveToFileLocked(s.lastKnownGoodPath, &s.config)
		} else {
			if saveErr := s.saveToFileLocked(cleaned, &s.config); saveErr != nil {
				return fmt.Errorf("failed to write config to new path %s: %w", cleaned, saveErr)
			}
		}
	}

	s.configPath = cleaned
	s.saveBootstrapLocked()
	return nil
}

func (s *Store) readConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid json in %s: %w", path, err)
	}

	norm := normalizeConfig(cfg)
	expanded := s.expandConfigPaths(norm)
	return &expanded, nil
}

func (s *Store) saveToFileLocked(path string, cfg *Config) error {
	toSave := s.foldConfigPaths(*cfg)

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return atomicWriteFile(path, data, 0644)
}

func (s *Store) saveBootstrapLocked() {
	boot := BootstrapConfig{ConfigPath: s.configPath}
	data, err := json.MarshalIndent(boot, "", "  ")
	if err == nil {
		_ = atomicWriteFile(s.bootstrapPath, data, 0644)
	}
}

func (s *Store) foldConfigPaths(cfg Config) Config {
	res := cloneConfig(cfg)
	for i, app := range cfg.Apps {
		if cfg.PortableEnvPaths {
			res.Apps[i].ExePath = FoldPath(app.ExePath)
		}
	}
	return res
}

func (s *Store) expandConfigPaths(cfg Config) Config {
	res := cloneConfig(cfg)
	for i, app := range cfg.Apps {
		if cfg.PortableEnvPaths {
			res.Apps[i].ExePath = ExpandPath(app.ExePath)
		}
	}
	return res
}

// FoldPath replaces system directory prefixes with %ENV% variables.
func FoldPath(path string) string {
	return foldPathWithEnv(path, os.Getenv)
}

// ExpandPath replaces %ENV% variables with system directory values.
func ExpandPath(path string) string {
	return expandPathWithEnv(path, os.Getenv)
}

func foldPathWithEnv(path string, getenv func(string) string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	normPath := strings.ReplaceAll(path, "/", "\\")

	type envPair struct {
		name string
		val  string
	}

	var pairs []envPair
	for _, varName := range defaultEnvVars {
		val := getenv(varName)
		if val != "" {
			normVal := strings.TrimRight(strings.ReplaceAll(val, "/", "\\"), "\\")
			if normVal != "" {
				pairs = append(pairs, envPair{name: varName, val: normVal})
			}
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return len(pairs[i].val) > len(pairs[j].val)
	})

	lowerPath := strings.ToLower(normPath)

	for _, pair := range pairs {
		lowerVal := strings.ToLower(pair.val)
		if lowerPath == lowerVal {
			return "%" + pair.name + "%"
		}
		if strings.HasPrefix(lowerPath, lowerVal) {
			if len(normPath) > len(pair.val) && normPath[len(pair.val)] == '\\' {
				rest := normPath[len(pair.val):]
				return "%" + pair.name + "%" + rest
			}
		}
	}

	return path
}

func expandPathWithEnv(path string, getenv func(string) string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	return envVarRegex.ReplaceAllStringFunc(path, func(match string) string {
		varName := match[1 : len(match)-1]
		val := getenv(varName)
		if val != "" {
			return val
		}
		return match
	})
}

func atomicWriteFile(targetPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	maxRetries := 5
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		tmpFile, err := os.CreateTemp(dir, ".split_tunnel_tmp_*")
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
			continue
		}
		tmpPath := tmpFile.Name()

		_, writeErr := tmpFile.Write(data)
		syncErr := tmpFile.Sync()
		closeErr := tmpFile.Close()

		if writeErr != nil || syncErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if writeErr != nil {
				lastErr = writeErr
			} else if syncErr != nil {
				lastErr = syncErr
			} else {
				lastErr = closeErr
			}
			time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
			continue
		}

		err = os.Rename(tmpPath, targetPath)
		if err == nil {
			return nil
		}

		_ = os.Remove(tmpPath)
		lastErr = err
		time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
	}

	err := os.WriteFile(targetPath, data, perm)
	if err != nil {
		return fmt.Errorf("atomic write and fallback write failed: last rename err: %v, write err: %w", lastErr, err)
	}

	return nil
}
