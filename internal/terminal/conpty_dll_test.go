//go:build windows

package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestLoadPseudoConsoleFallsBackToInbox(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, BundledConPTYLibrary)
	cases := map[string]struct {
		setting   string
		directory string
	}{
		"forced inbox":            {setting: "INBOX", directory: directory},
		"no executable directory": {},
		"missing bundled library": {directory: directory},
		"missing configured path": {setting: missing},
		"relative configured":     {setting: BundledConPTYLibrary},
	}
	for name, c := range cases {
		if got := loadPseudoConsole(c.setting, c.directory); got.name != inboxPseudoConsole.name {
			t.Fatalf("%s: loadPseudoConsole = %q, want inbox", name, got.name)
		}
	}
}

func TestLoadPseudoConsoleRejectsLibraryWithoutConPTYExports(t *testing.T) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if got := loadPseudoConsole(filepath.Join(system, "version.dll"), ""); got.name != inboxPseudoConsole.name {
		t.Fatalf("loadPseudoConsole(version.dll) = %q, want inbox", got.name)
	}
}

func TestHresultError(t *testing.T) {
	if err := hresultError(0); err != nil {
		t.Fatalf("S_OK returned %v", err)
	}
	if err := hresultError(1); err != nil {
		t.Fatalf("S_FALSE returned %v", err)
	}
	if err := hresultError(uintptr(0x80070057)); err == nil {
		t.Fatal("E_INVALIDARG returned nil")
	}
}

// Runs when PANEACEA_CONPTY points at the bundled conpty.dll (with OpenConsole.exe beside it).
func TestBundledConPTYPassesMouseModesThrough(t *testing.T) {
	library := os.Getenv(ConPTYEnvironmentVariable)
	if library == "" || strings.EqualFold(library, "inbox") {
		t.Skip("set " + ConPTYEnvironmentVariable + " to the bundled conpty.dll to run")
	}
	if PseudoConsoleImplementation() != library {
		t.Fatalf("pseudo console = %q, want %q", PseudoConsoleImplementation(), library)
	}
	output := make(chan string, 64)
	session, err := NewSession(Options{
		Executable: "cmd.exe",
		Arguments:  []string{"/d", "/c", "echo \x1b[?1003hPANEACEA-MOUSE & ping -n 2 127.0.0.1 >nul"},
		Columns:    80,
		Rows:       24,
		OnOutput:   func(data string) { output <- data },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var received strings.Builder
	timeout := time.After(10 * time.Second)
	for !strings.Contains(received.String(), "PANEACEA-MOUSE") {
		select {
		case data := <-output:
			received.WriteString(data)
		case <-timeout:
			t.Fatalf("timed out; output = %q", received.String())
		}
	}
	if !strings.Contains(received.String(), "\x1b[?1003h") {
		t.Fatalf("mouse mode was not passed through: %q", received.String())
	}
}
