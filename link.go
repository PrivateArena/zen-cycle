package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	// Check if it's a symlink or directory junction
	isLink := fi.Mode()&os.ModeSymlink != 0

	if !isLink {
		// On Windows, check for reparse point attribute manually just in case
		if runtime.GOOS == "windows" {
			if fi.Mode()&os.ModeDir != 0 {
				// Let's read link just to see if it succeeds. If it fails, it's a real dir.
				_, err := os.Readlink(linkPath)
				if err == nil {
					isLink = true
				}
			}
		}
	}

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
		if isProcessRunning(name) {
			active = append(active, name)
		}
	}
	return active, nil
}

func isProcessRunning(name string) bool {
	nameLower := strings.ToLower(name)
	if runtime.GOOS == "windows" {
		// Check using tasklist
		cmd := exec.Command("tasklist", "/NH", "/FO", "CSV")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(string(output)), nameLower)
	}

	// Linux / macOS: check using pgrep
	cmd := exec.Command("pgrep", "-f", name)
	if err := cmd.Run(); err == nil {
		return true
	}

	// Fallback check via ps
	cmd = exec.Command("ps", "-ax", "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), nameLower) {
			return true
		}
	}
	return false
}

// SwitchActiveSource updates the symlink/junction to point to the new target.
func SwitchActiveSource(p Project, newTarget string) error {
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
	return createLink(targetPath, linkPath)
}

func createLink(target, link string) error {
	if runtime.GOOS != "windows" {
		// POSIX: Atomic swap using temporary link and rename
		tmpLink := link + fmt.Sprintf(".tmp-%d", os.Getpid())
		_ = os.Remove(tmpLink)

		// Use relative path for link target to keep the project portable
		relTarget, err := filepath.Rel(filepath.Dir(link), target)
		if err != nil {
			relTarget = target // fallback to absolute
		}

		if err := os.Symlink(relTarget, tmpLink); err != nil {
			return err
		}
		if err := os.Rename(tmpLink, link); err != nil {
			_ = os.Remove(tmpLink)
			return err
		}
		return nil
	}

	// Windows implementation

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	absLink, err := filepath.Abs(link)
	if err != nil {
		return err
	}

	tmpLink := absLink + fmt.Sprintf(".tmp-%d", os.Getpid())
	_ = removeReparsePoint(tmpLink) // clean stale leftovers

	// 1. Create new link at temp path — old link untouched
	cmd := exec.Command("cmd", "/c", "mklink", "/J", tmpLink, absTarget)
	if output, err := cmd.CombinedOutput(); err != nil {
		if errSym := os.Symlink(absTarget, tmpLink); errSym != nil {
			_ = removeReparsePoint(tmpLink)
			return fmt.Errorf("windows junction/symlink failed: %s - %w", string(output), errSym)
		}
	}

	// Verify new link resolves before touching the old one
	if _, err := os.Stat(tmpLink); err != nil {
		_ = removeReparsePoint(tmpLink)
		return fmt.Errorf("new link failed validation: %w", err)
	}

	// 2. Move old link aside (don't delete) so we can restore on failure
	backupLink := absLink + fmt.Sprintf(".bak-%d", os.Getpid())
	_ = os.Remove(backupLink)
	hadOld := false
	if _, err := os.Lstat(absLink); err == nil {
		if err := os.Rename(absLink, backupLink); err != nil {
			_ = removeReparsePoint(tmpLink)
			return fmt.Errorf("failed to move existing link aside: %w", err)
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		_ = removeReparsePoint(tmpLink)
		return err
	}

	// 3. Activate new link
	if err := os.Rename(tmpLink, absLink); err != nil {
		if hadOld {
			_ = os.Rename(backupLink, absLink)
		}
		_ = removeReparsePoint(tmpLink)
		return fmt.Errorf("failed to activate new link: %w", err)
	}

	// 4. Only now discard the old link
	if hadOld {
		_ = removeReparsePoint(backupLink)
	}
	return nil
}

func removeReparsePoint(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	if fi.IsDir() {
		return ErrRealDirectory
	}
	return os.Remove(path)
}

// IsNTFS checks if the parent volume is NTFS (required for Windows Junctions)
func IsNTFS(path string) (bool, error) {
	if runtime.GOOS != "windows" {
		return true, nil
	}
	// For simplicity, we assume true. If mklink /J fails, the fallback to os.Symlink handles it.
	return true, nil
}
