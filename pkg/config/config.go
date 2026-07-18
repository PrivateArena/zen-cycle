package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Project represents a folder cycling configuration.
type Project struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`             // Path to the project root folder
	SymlinkName     string   `json:"symlink_name"`     // e.g. "profile"
	CurrentActive   string   `json:"current_active"`   // The name of the currently selected target within .zen-cycle
	ProcessDenylist []string `json:"process_denylist"` // Process names to guard against split-brain writes
}

// Config wraps all application settings.
type Config struct {
	Projects    []Project `json:"projects"`
	ScrollSpeed float64   `json:"scroll_speed"` // Scroll multiplier for profile list (1.0 = default)
}

var (
	ConfigMu   sync.Mutex
	LoadedPath string
)

func getBinaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		dir, _ := os.Getwd()
		return dir
	}
	dir := filepath.Dir(exe)
	if strings.Contains(exe, "go-build") || strings.Contains(dir, "Temp") {
		dir, _ = os.Getwd()
	}
	return dir
}

// getConfigPaths returns candidate paths in priority order.
func getConfigPaths() []string {
	binDir := getBinaryDir()
	cwd, _ := os.Getwd()

	var paths []string
	if LoadedPath != "" {
		paths = append(paths, LoadedPath)
	}

	paths = append(paths,
		filepath.Join(binDir, "config.json"),
		filepath.Join(cwd, "config.json"),
	)

	userConfig, err := os.UserConfigDir()
	if err == nil {
		paths = append(paths, filepath.Join(userConfig, "zen-cycle", "config.json"))
	}
	return paths
}

// LoadConfig reads configuration using portable priority rules.
func LoadConfig() (*Config, error) {
	ConfigMu.Lock()
	defer ConfigMu.Unlock()

	paths := getConfigPaths()
	var finalErr error

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			data, err := os.ReadFile(absPath)
			if err != nil {
				finalErr = err
				continue
			}
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				// Backup corrupt file and return error
				_ = backupCorruptConfig(absPath)
				return nil, fmt.Errorf("config corrupt, backed up: %w", err)
			}
			if cfg.ScrollSpeed <= 0 {
				cfg.ScrollSpeed = 1.0
			}
			LoadedPath = absPath
			log.Printf("[Config] Loaded from: %s", absPath)
			return &cfg, nil
		}
	}

	// Default config if none exists
	defaultCfg := &Config{Projects: []Project{}, ScrollSpeed: 1.0}
	if LoadedPath == "" {
		LoadedPath = filepath.Join(getBinaryDir(), "config.json")
	}
	return defaultCfg, finalErr
}

// SaveConfigAtomic saves configuration atomically.
func SaveConfigAtomic(cfg *Config) error {
	ConfigMu.Lock()
	defer ConfigMu.Unlock()

	if LoadedPath == "" {
		LoadedPath = filepath.Join(getBinaryDir(), "config.json")
	}

	dir := filepath.Dir(LoadedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	// Atomic rename. On Windows, standard os.Rename can fail if dest is open.
	if runtime.GOOS == "windows" {
		_ = os.Remove(LoadedPath) // Remove destination first to ensure overwrite compatibility
	}

	if err := os.Rename(tmpName, LoadedPath); err != nil {
		return fmt.Errorf("failed replacing config file: %w", err)
	}

	log.Printf("[Config] Atomic save completed to: %s", LoadedPath)
	return nil
}

func backupCorruptConfig(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	bakPath := path + ".bak"
	dst, err := os.Create(bakPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
