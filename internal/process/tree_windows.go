package process

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

type Info struct {
	PID        uint32
	ParentPID  uint32
	Executable string
	Generation string
}

func Snapshot() ([]Info, error) {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	items := []Info{}
	err = windows.Process32First(handle, &entry)
	for err == nil {
		items = append(items, Info{PID: entry.ProcessID, ParentPID: entry.ParentProcessID, Executable: windows.UTF16ToString(entry.ExeFile[:])})
		err = windows.Process32Next(handle, &entry)
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, err
	}
	return items, nil
}
func Generation(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", created.HighDateTime, created.LowDateTime)
}
func Descendants(items []Info, root uint32) []Info {
	result := []Info{}
	seen := map[uint32]bool{root: true}
	for {
		added := false
		for _, p := range items {
			if !seen[p.PID] && seen[p.ParentPID] {
				seen[p.PID] = true
				result = append(result, p)
				added = true
			}
		}
		if !added {
			return result
		}
	}
}
