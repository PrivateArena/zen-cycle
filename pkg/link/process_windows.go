package link

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listRunningProcessNames walks a CreateToolhelp32Snapshot process list
// instead of shelling out to tasklist.exe.
func listRunningProcessNames() ([]string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	var names []string
	if err := windows.Process32First(snapshot, &entry); err != nil {
		if err == syscall.ERROR_NO_MORE_FILES {
			return names, nil
		}
		return nil, err
	}
	for {
		names = append(names, windows.UTF16ToString(entry.ExeFile[:]))
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == syscall.ERROR_NO_MORE_FILES {
				break
			}
			return nil, err
		}
	}
	return names, nil
}
