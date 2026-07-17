package main

import (
	"golang.org/x/sys/unix"
)

// listRunningProcessNames enumerates the process table via the
// CTL_KERN/KERN_PROC/KERN_PROC_ALL sysctl (wrapped by x/sys/unix as
// SysctlKinfoProcSlice) instead of shelling out to ps/pgrep.
func listRunningProcessNames() ([]string, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(procs))
	for _, p := range procs {
		comm := p.Proc.P_comm[:]
		// P_comm is a fixed-size, NUL-terminated (and padded) byte array.
		end := len(comm)
		for i, b := range comm {
			if b == 0 {
				end = i
				break
			}
		}
		if end > 0 {
			names = append(names, string(comm[:end]))
		}
	}
	return names, nil
}
