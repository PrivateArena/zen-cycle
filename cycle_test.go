package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicConfig(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Set loadedPath to temp directory for testing
	loadedPath = filepath.Join(tmpDir, "config.json")
	
	cfg := &Config{
		Projects: []Project{
			{
				Name:        "Test Project",
				Path:        tmpDir,
				SymlinkName: "profile",
			},
		},
	}

	// Save
	if err := SaveConfigAtomic(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(loadedPath); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}

	// Load
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "Test Project" {
		t.Errorf("loaded config does not match saved data: %+v", loaded)
	}
}

func TestSymlinkJunction(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a dummy .zen-cycle structure
	zenCycleDir := filepath.Join(tmpDir, ".zen-cycle")
	if err := os.MkdirAll(zenCycleDir, 0755); err != nil {
		t.Fatal(err)
	}

	profileADir := filepath.Join(zenCycleDir, "profile_A")
	if err := os.MkdirAll(profileADir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a dummy file in profile_A
	dummyFile := filepath.Join(profileADir, "data.txt")
	if err := os.WriteFile(dummyFile, []byte("hello A"), 0644); err != nil {
		t.Fatal(err)
	}

	// Define project
	p := Project{
		Name:        "Test Project",
		Path:        tmpDir,
		SymlinkName: "profile",
	}

	// Switch to profile_A
	if err := SwitchActiveSource(p, "profile_A"); err != nil {
		t.Fatalf("failed to switch source: %v", err)
	}

	// Verify link exists and points to profile_A
	linkPath := filepath.Join(tmpDir, "profile")
	active, err := DetectActiveSource(p.Path, p.SymlinkName)
	if err != nil {
		t.Fatalf("failed to detect active source: %v", err)
	}

	if active != "profile_A" {
		t.Errorf("expected active to be 'profile_A', got %q", active)
	}

	// Verify file can be read through link
	linkedFile := filepath.Join(linkPath, "data.txt")
	data, err := os.ReadFile(linkedFile)
	if err != nil {
		t.Fatalf("failed to read file through link: %v", err)
	}

	if string(data) != "hello A" {
		t.Errorf("expected data to be 'hello A', got %q", string(data))
	}

	// Verify cycle sources detection
	sources, err := GetCycleSources(tmpDir)
	if err != nil {
		t.Fatalf("failed to get cycle sources: %v", err)
	}

	if len(sources) != 1 || sources[0] != "profile_A" {
		t.Errorf("expected cycle sources to be ['profile_A'], got %v", sources)
	}

	// Verify safety check for real directory
	// If link is replaced with a real directory, SwitchActiveSource should fail
	if err := removeReparsePoint(linkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkPath, 0755); err != nil {
		t.Fatal(err)
	}

	err = SwitchActiveSource(p, "profile_A")
	if err == nil {
		t.Error("expected error when target is a real directory, got nil")
	}

	// Cleanup directory
	_ = os.RemoveAll(linkPath)
}

func TestProcessRunningCheck(t *testing.T) {
	// Querying a non-existent process should return false
	running := isProcessRunning("nonexistent_process_123456.exe")
	if running {
		t.Error("expected nonexistent process to not be running")
	}

	// Querying a process denylist check
	active, err := CheckDenylistProcesses([]string{"nonexistent_process_123456.exe"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) > 0 {
		t.Errorf("expected no active processes, got %v", active)
	}
}

func TestWindowsReparseRemoval(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific reparse point removal test")
	}
	// This test specifically executes on Windows to verify junction removal behavior
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target")
	link := filepath.Join(tmpDir, "link")
	_ = os.MkdirAll(target, 0755)

	err := createLink(target, link)
	if err != nil {
		t.Fatalf("failed to create Windows junction/link: %v", err)
	}

	err = removeReparsePoint(link)
	if err != nil {
		t.Fatalf("failed to remove Windows junction/link: %v", err)
	}

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("expected link to be deleted")
	}
}
