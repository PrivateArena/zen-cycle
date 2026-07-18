package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"zen-cycle/pkg/config"
)

func TestAtomicConfig(t *testing.T) {
	tmpDir := t.TempDir()

	config.LoadedPath = filepath.Join(tmpDir, "config.json")

	cfg := &config.Config{
		Projects: []config.Project{
			{
				Name:        "Test Project",
				Path:        tmpDir,
				SymlinkName: "profile",
			},
		},
	}

	if err := config.SaveConfigAtomic(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if _, err := os.Stat(config.LoadedPath); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}

	loaded, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "Test Project" {
		t.Errorf("loaded config does not match saved data: %+v", loaded)
	}
}
