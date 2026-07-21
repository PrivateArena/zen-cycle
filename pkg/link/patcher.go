package link

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// ErrNoPatcher indicates that .cycle-patcher is absent. Callers should treat
// this as a no-op (the feature is opt-in via directory convention).
var ErrNoPatcher = errors.New(".cycle-patcher not present (no-op)")

// PatcherReport summarises a single PatchProfile invocation.
type PatcherReport struct {
	Applied []string // relative paths successfully symlinked
	Skipped []string // relative paths where dst already existed
	Errors  []string // "rel: err" entries that failed (non-fatal)
}

// PatchProfile symlinks every file under .zen-cycle/.cycle-patcher/ into the
// given profile directory, mirroring relative structure. It is idempotent and
// non-destructive: existing destinations are skipped, never deleted.
//
// The patcher is best-effort. Per-file failures are recorded in the returned
// report's Errors slice rather than aborting the walk. A returned error is
// non-nil only for fatal conditions (missing patcher root → ErrNoPatcher;
// unreadable patcher root → wrapped fs error).
func PatchProfile(projectPath, profileName string) (PatcherReport, error) {
	var report PatcherReport

	patcherRoot := filepath.Join(projectPath, ".zen-cycle", ".cycle-patcher")
	if _, err := os.Stat(patcherRoot); err != nil {
		if os.IsNotExist(err) {
			return report, ErrNoPatcher
		}
		return report, fmt.Errorf("patcher root unreadable: %w", err)
	}

	profileRoot := filepath.Join(projectPath, ".zen-cycle", profileName)
	if err := os.MkdirAll(profileRoot, 0755); err != nil {
		return report, fmt.Errorf("profile root unusable: %w", err)
	}

	err := filepath.WalkDir(patcherRoot, func(srcAbs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			rel, _ := filepath.Rel(patcherRoot, srcAbs)
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", rel, walkErr))
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil // parents are created on-demand per file
		}

		rel, err := filepath.Rel(patcherRoot, srcAbs)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", srcAbs, err))
			return nil
		}
		// Path-traversal guard: reject absolute or escaping relative paths.
		if !filepath.IsLocal(rel) {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: path escapes patcher root", rel))
			return nil
		}

		dst := filepath.Join(profileRoot, rel)

		// Existing destination (file, dir, or symlink incl. broken) → skip.
		if _, err := os.Lstat(dst); err == nil {
			report.Skipped = append(report.Skipped, rel)
			return nil
		} else if !os.IsNotExist(err) {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", rel, err))
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", rel, err))
			return nil
		}

		if err := CreateLink(srcAbs, dst); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", rel, err))
			return nil
		}

		report.Applied = append(report.Applied, rel)
		return nil
	})
	if err != nil {
		return report, fmt.Errorf("patcher walk failed: %w", err)
	}

	log.Printf("[Patcher] profile=%s applied=%d skipped=%d errors=%d",
		profileName, len(report.Applied), len(report.Skipped), len(report.Errors))
	return report, nil
}
