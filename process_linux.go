package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// listRunningProcessNames reads /proc directly instead of shelling out to
// pgrep/ps. Returns both the short "comm" name and the argv[0] basename for
// each running process, since denylist entries might match either style.
func listRunningProcessNames() ([]string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue // not a pid directory
		}
		pidDir := "/proc/" + e.Name()

		if comm, err := os.ReadFile(pidDir + "/comm"); err == nil {
			if n := strings.TrimSpace(string(comm)); n != "" {
				names = append(names, n)
			}
		}

		if cmdline, err := os.ReadFile(pidDir + "/cmdline"); err == nil && len(cmdline) > 0 {
			argv0 := strings.SplitN(string(cmdline), "\x00", 2)[0]
			if argv0 != "" {
				names = append(names, filepath.Base(argv0))
			}
		}
	}
	return names, nil
}
