package link_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"zen-cycle/pkg/config"
	"zen-cycle/pkg/link"
)

func TestSymlinkJunction(t *testing.T) {
	tmpDir := t.TempDir()

	zenCycleDir := filepath.Join(tmpDir, ".zen-cycle")
	if err := os.MkdirAll(zenCycleDir, 0755); err != nil {
		t.Fatal(err)
	}

	profileADir := filepath.Join(zenCycleDir, "profile_A")
	if err := os.MkdirAll(profileADir, 0755); err != nil {
		t.Fatal(err)
	}

	dummyFile := filepath.Join(profileADir, "data.txt")
	if err := os.WriteFile(dummyFile, []byte("hello A"), 0644); err != nil {
		t.Fatal(err)
	}

	p := config.Project{
		Name:        "Test Project",
		Path:        tmpDir,
		SymlinkName: "profile",
	}

	if err := link.SwitchActiveSource(p, "profile_A"); err != nil {
		t.Fatalf("failed to switch source: %v", err)
	}

	linkPath := filepath.Join(tmpDir, "profile")
	active, err := link.DetectActiveSource(p.Path, p.SymlinkName)
	if err != nil {
		t.Fatalf("failed to detect active source: %v", err)
	}

	if active != "profile_A" {
		t.Errorf("expected active to be 'profile_A', got %q", active)
	}

	linkedFile := filepath.Join(linkPath, "data.txt")
	data, err := os.ReadFile(linkedFile)
	if err != nil {
		t.Fatalf("failed to read file through link: %v", err)
	}

	if string(data) != "hello A" {
		t.Errorf("expected data to be 'hello A', got %q", string(data))
	}

	sources, err := link.GetCycleSources(tmpDir)
	if err != nil {
		t.Fatalf("failed to get cycle sources: %v", err)
	}

	if len(sources) != 1 || sources[0] != "profile_A" {
		t.Errorf("expected cycle sources to be ['profile_A'], got %v", sources)
	}

	_ = os.Remove(linkPath)
	if err := os.MkdirAll(linkPath, 0755); err != nil {
		t.Fatal(err)
	}

	err = link.SwitchActiveSource(p, "profile_A")
	if err == nil {
		t.Error("expected error when target is a real directory, got nil")
	}

	_ = os.RemoveAll(linkPath)
}

func TestProcessRunningCheck(t *testing.T) {
	running := link.IsProcessRunning("nonexistent_process_123456.exe")
	if running {
		t.Error("expected nonexistent process to not be running")
	}

	active, err := link.CheckDenylistProcesses([]string{"nonexistent_process_123456.exe"})
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
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target")
	linkPath := filepath.Join(tmpDir, "link")
	_ = os.MkdirAll(target, 0755)

	err := link.CreateLink(target, linkPath)
	if err != nil {
		t.Fatalf("failed to create Windows junction/link: %v", err)
	}

	err = os.Remove(linkPath)
	if err != nil {
		t.Fatalf("failed to remove Windows junction/link: %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("expected link to be deleted")
	}
}
