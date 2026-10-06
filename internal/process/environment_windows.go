package process

import (
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// maxEnvironmentSize caps how much of a remote environment block is read.
const maxEnvironmentSize = 1 << 20

// EnvironmentVariable reads name from the environment of pid. It reads the
// target's PEB with read-only access (PROCESS_VM_READ) and only supports
// processes with the same bitness as the caller. ok is false if the
// environment could not be read.
func EnvironmentVariable(pid uint32, name string) (value string, ok bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(handle)
	var self, target bool
	if windows.IsWow64Process(windows.CurrentProcess(), &self) != nil || windows.IsWow64Process(handle, &target) != nil || self != target {
		return "", false
	}
	var info windows.PROCESS_BASIC_INFORMATION
	if windows.NtQueryInformationProcess(handle, windows.ProcessBasicInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), nil) != nil || info.PebBaseAddress == nil {
		return "", false
	}
	var peb windows.PEB
	if !readRemote(handle, uintptr(unsafe.Pointer(info.PebBaseAddress)), unsafe.Pointer(&peb), unsafe.Sizeof(peb)) || peb.ProcessParameters == nil {
		return "", false
	}
	var params windows.RTL_USER_PROCESS_PARAMETERS
	if !readRemote(handle, uintptr(unsafe.Pointer(peb.ProcessParameters)), unsafe.Pointer(&params), unsafe.Sizeof(params)) || params.Environment == nil {
		return "", false
	}
	size := params.EnvironmentSize
	if size == 0 || size > maxEnvironmentSize {
		size = maxEnvironmentSize
	}
	block := make([]uint16, size/2)
	if len(block) == 0 || !readRemote(handle, uintptr(params.Environment), unsafe.Pointer(&block[0]), uintptr(len(block)*2)) {
		return "", false
	}
	value, _ = lookupEnvironmentBlock(block, name)
	return value, true
}

func readRemote(handle windows.Handle, address uintptr, buffer unsafe.Pointer, size uintptr) bool {
	var read uintptr
	return windows.ReadProcessMemory(handle, address, (*byte)(buffer), size, &read) == nil && read == size
}

// lookupEnvironmentBlock finds name (case-insensitive) in a double-NUL
// terminated UTF-16 environment block.
func lookupEnvironmentBlock(block []uint16, name string) (string, bool) {
	prefix := strings.ToUpper(name) + "="
	for start := 0; start < len(block); {
		end := start
		for end < len(block) && block[end] != 0 {
			end++
		}
		if end == start {
			break
		}
		entry := windows.UTF16ToString(block[start:end])
		if len(entry) >= len(prefix) && strings.ToUpper(entry[:len(prefix)]) == prefix {
			return entry[len(prefix):], true
		}
		start = end + 1
	}
	return "", false
}

// environmentCache stores successful lookups per PID and process generation.
// Failed reads are not cached so a process that is still starting is retried.
var environmentCache = struct {
	sync.Mutex
	items map[uint32]cachedCommandLine
}{items: map[uint32]cachedCommandLine{}}

// CachedEnvironmentVariable returns EnvironmentVariable(pid, name), reusing a
// cached value while the process generation is unchanged. Only one variable
// name is expected to be cached per process.
func CachedEnvironmentVariable(pid uint32, generation, name string) string {
	environmentCache.Lock()
	cached, ok := environmentCache.items[pid]
	environmentCache.Unlock()
	if ok && cached.generation == generation {
		return cached.commandLine
	}
	value, ok := EnvironmentVariable(pid, name)
	if !ok {
		return ""
	}
	environmentCache.Lock()
	environmentCache.items[pid] = cachedCommandLine{generation: generation, commandLine: value}
	environmentCache.Unlock()
	return value
}

// ImagePath returns the full executable path of pid, or "" if unavailable.
func ImagePath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(handle, 0, &buf[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// IsMSYSImage reports whether path is an MSYS/Git Bash program (under a
// usr\bin directory). Only these lose their parent through fork/exec.
func IsMSYSImage(path string) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(path, "/", `\`)), `\usr\bin\`)
}

// Orphans returns processes whose parent is no longer running, e.g. children
// left behind by Git Bash/MSYS fork+exec.
func Orphans(items []Info) []Info {
	live := make(map[uint32]bool, len(items))
	for _, p := range items {
		live[p.PID] = true
	}
	result := []Info{}
	for _, p := range items {
		if p.PID != 0 && p.ParentPID != 0 && !live[p.ParentPID] {
			result = append(result, p)
		}
	}
	return result
}
