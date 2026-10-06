package process

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CommandLine returns the command line of pid, or "" if it cannot be read.
// It uses ProcessCommandLineInformation (Windows 8.1+), which only needs
// PROCESS_QUERY_LIMITED_INFORMATION and does not read the target's memory.
func CommandLine(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	size := uint32(4096)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		var needed uint32
		err = windows.NtQueryInformationProcess(handle, windows.ProcessCommandLineInformation, unsafe.Pointer(&buf[0]), size, &needed)
		if err == nil {
			return (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0])).String()
		}
		if needed <= size {
			return ""
		}
		size = needed
	}
	return ""
}

type cachedCommandLine struct {
	generation  string
	commandLine string
}

// commandLineCache keeps command lines per PID and process generation so the
// monitor poll does not re-query unchanged processes.
var commandLineCache = struct {
	sync.Mutex
	items map[uint32]cachedCommandLine
}{items: map[uint32]cachedCommandLine{}}

// CachedCommandLine returns the command line of pid, reusing a cached value
// while the process generation is unchanged.
func CachedCommandLine(pid uint32, generation string) string {
	commandLineCache.Lock()
	cached, ok := commandLineCache.items[pid]
	commandLineCache.Unlock()
	if ok && cached.generation == generation {
		return cached.commandLine
	}
	commandLine := CommandLine(pid)
	commandLineCache.Lock()
	commandLineCache.items[pid] = cachedCommandLine{generation: generation, commandLine: commandLine}
	commandLineCache.Unlock()
	return commandLine
}

// PruneCommandLines drops cached command line and environment entries for
// processes no longer in items.
func PruneCommandLines(items []Info) {
	live := make(map[uint32]bool, len(items))
	for _, p := range items {
		live[p.PID] = true
	}
	commandLineCache.Lock()
	for pid := range commandLineCache.items {
		if !live[pid] {
			delete(commandLineCache.items, pid)
		}
	}
	commandLineCache.Unlock()
	environmentCache.Lock()
	for pid := range environmentCache.items {
		if !live[pid] {
			delete(environmentCache.items, pid)
		}
	}
	environmentCache.Unlock()
}
