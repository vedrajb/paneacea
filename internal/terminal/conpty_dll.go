//go:build windows

package terminal

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ConPTYEnvironmentVariable selects the pseudo console implementation: "inbox" forces the
// Windows kernel32 ConPTY, a path selects a specific conpty.dll, and empty uses the conpty.dll
// bundled beside the runtime executable when present.
const ConPTYEnvironmentVariable = "PANEACEA_CONPTY"

// BundledConPTYLibrary is the Microsoft.Windows.Console.ConPTY library shipped with the runtime.
// Unlike the inbox ConPTY, it passes application VT modes (mouse tracking, etc.) to the terminal.
const BundledConPTYLibrary = "conpty.dll"

type pseudoConsoleAPI struct {
	name   string
	create func(size windows.Coord, input windows.Handle, output windows.Handle, flags uint32, pseudo *windows.Handle) error
	resize func(pseudo windows.Handle, size windows.Coord) error
	close  func(pseudo windows.Handle)
}

var inboxPseudoConsole = pseudoConsoleAPI{
	name:   "inbox",
	create: windows.CreatePseudoConsole,
	resize: windows.ResizePseudoConsole,
	close:  windows.ClosePseudoConsole,
}

var (
	pseudoConsoleOnce sync.Once
	pseudoConsole     pseudoConsoleAPI
)

func currentPseudoConsole() pseudoConsoleAPI {
	pseudoConsoleOnce.Do(func() {
		directory := ""
		if executable, err := os.Executable(); err == nil {
			directory = filepath.Dir(executable)
		}
		pseudoConsole = loadPseudoConsole(os.Getenv(ConPTYEnvironmentVariable), directory)
		log.Printf("terminal: using %s pseudo console", pseudoConsole.name)
	})
	return pseudoConsole
}

// PseudoConsoleImplementation names the ConPTY in use: "inbox" or the bundled conpty.dll path.
func PseudoConsoleImplementation() string {
	return currentPseudoConsole().name
}

func loadPseudoConsole(setting string, directory string) pseudoConsoleAPI {
	if strings.EqualFold(setting, "inbox") {
		return inboxPseudoConsole
	}
	path := setting
	if path == "" {
		if directory == "" {
			return inboxPseudoConsole
		}
		path = filepath.Join(directory, BundledConPTYLibrary)
	}
	if !filepath.IsAbs(path) {
		log.Printf("terminal: ignoring relative %s path %q", ConPTYEnvironmentVariable, path)
		return inboxPseudoConsole
	}
	if _, err := os.Stat(path); err != nil {
		if setting != "" {
			log.Printf("terminal: %s library unavailable: %v", ConPTYEnvironmentVariable, err)
		}
		return inboxPseudoConsole
	}
	library := windows.NewLazyDLL(path)
	create := library.NewProc("ConptyCreatePseudoConsole")
	resize := library.NewProc("ConptyResizePseudoConsole")
	closeConsole := library.NewProc("ConptyClosePseudoConsole")
	for _, procedure := range []*windows.LazyProc{create, resize, closeConsole} {
		if err := procedure.Find(); err != nil {
			log.Printf("terminal: bundled ConPTY %s is unusable: %v", path, err)
			return inboxPseudoConsole
		}
	}
	return pseudoConsoleAPI{
		name: path,
		create: func(size windows.Coord, input windows.Handle, output windows.Handle, flags uint32, pseudo *windows.Handle) error {
			result, _, _ := create.Call(uintptr(coordValue(size)), uintptr(input), uintptr(output), uintptr(flags), uintptr(unsafe.Pointer(pseudo)))
			return hresultError(result)
		},
		resize: func(pseudo windows.Handle, size windows.Coord) error {
			result, _, _ := resize.Call(uintptr(pseudo), uintptr(coordValue(size)))
			return hresultError(result)
		},
		close: func(pseudo windows.Handle) {
			_, _, _ = closeConsole.Call(uintptr(pseudo))
		},
	}
}

// coordValue packs a COORD the way it is passed by value to the ConPTY functions.
func coordValue(size windows.Coord) uint32 {
	return *(*uint32)(unsafe.Pointer(&size))
}

func hresultError(result uintptr) error {
	if int32(result) >= 0 {
		return nil
	}
	return syscall.Errno(uint32(result))
}
