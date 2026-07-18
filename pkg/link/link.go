package link

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zen-cycle/pkg/config"
)

var (
	ErrRealDirectory = errors.New("target path exists and is a real directory (not a link)")
)

// GetCycleSources reads all entries inside .zen-cycle directory.
func GetCycleSources(projectPath string) ([]string, error) {
	zenCycleDir := filepath.Join(projectPath, ".zen-cycle")
	entries, err := os.ReadDir(zenCycleDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Auto-create .zen-cycle if missing, so user can place sources
			_ = os.MkdirAll(zenCycleDir, 0755)
			return []string{}, nil
		}
		return nil, err
	}

	var sources []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sources = append(sources, entry.Name())
	}
	return sources, nil
}

// DetectActiveSource reads the link and checks where it points.
func DetectActiveSource(projectPath, symlinkName string) (string, error) {
	linkPath := filepath.Join(projectPath, symlinkName)
	fi, err := os.Lstat(linkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Not created yet
		}
		return "", err
	}

	// Since createLink always produces a real os.Symlink (on every OS,
	// including Windows via os.Symlink's directory-symlink support),
	// a plain ModeSymlink check is now sufficient everywhere.
	isLink := fi.Mode()&os.ModeSymlink != 0

	if !isLink {
		if fi.IsDir() {
			return "", ErrRealDirectory
		}
		return "", fmt.Errorf("target exists but is a regular file")
	}

	// Read target link
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "", err
	}

	// Resolve to basename
	base := filepath.Base(target)
	return base, nil
}

// CheckDenylistProcesses checks if any of the blacklisted processes are running.
func CheckDenylistProcesses(denylist []string) ([]string, error) {
	var active []string
	for _, p := range denylist {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		if IsProcessRunning(name) {
			active = append(active, name)
		}
	}
	return active, nil
}

// isProcessRunning delegates to a per-OS implementation (process_linux.go,
// process_darwin.go, process_windows.go) that reads the process list via
// direct syscalls — no shelling out to tasklist/pgrep/ps.
func IsProcessRunning(name string) bool {
	nameLower := strings.ToLower(name)
	procNames, err := listRunningProcessNames()
	if err != nil {
		return false
	}
	for _, p := range procNames {
		if strings.Contains(strings.ToLower(p), nameLower) {
			return true
		}
	}
	return false
}

// SwitchActiveSource updates the symlink to point to the new target.
func SwitchActiveSource(p config.Project, newTarget string) error {
	// 1. Process Guard
	activeProcesses, err := CheckDenylistProcesses(p.ProcessDenylist)
	if err != nil {
		return fmt.Errorf("process guard check failed: %w", err)
	}
	if len(activeProcesses) > 0 {
		return fmt.Errorf("active processes detected: %s. Close them first.", strings.Join(activeProcesses, ", "))
	}

	linkPath := filepath.Join(p.Path, p.SymlinkName)
	targetPath := filepath.Join(p.Path, ".zen-cycle", newTarget)

	// Check if target exists
	if _, err := os.Stat(targetPath); err != nil {
		return fmt.Errorf("target profile %q does not exist: %w", newTarget, err)
	}

	// 2. Perform link creation
	return CreateLink(targetPath, linkPath)
}

// createLink atomically points link -> target using a real symlink on every
// platform. Since Go 1.6, os.Symlink can create directory symlinks on
// Windows too (requires either Developer Mode enabled or an elevated
// process), so there's no need to shell out to "mklink /J" and no need to
// distinguish junctions from symlinks elsewhere in this file.
//
// The swap is atomic: build the new link at a temp path, verify it, then
// os.Rename it over the old link in one step. Rename-over-existing-symlink
// works on Linux, macOS, and Windows (NTFS) alike.
func CreateLink(target, link string) error {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	absLink, err := filepath.Abs(link)
	if err != nil {
		return err
	}

	tmpLink := absLink + fmt.Sprintf(".tmp-%d", os.Getpid())
	_ = os.Remove(tmpLink) // clean up any stale leftover from a crashed run

	// Prefer a relative target so the project directory stays portable
	// (e.g. if it's moved or synced elsewhere).
	linkTarget := absTarget
	if rel, err := filepath.Rel(filepath.Dir(absLink), absTarget); err == nil {
		linkTarget = rel
	}

	if err := os.Symlink(linkTarget, tmpLink); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Verify the new link resolves before touching the old one.
	if _, err := os.Stat(tmpLink); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("new link failed validation: %w", err)
	}

	if err := os.Rename(tmpLink, absLink); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("failed to activate new link: %w", err)
	}
	return nil
}
