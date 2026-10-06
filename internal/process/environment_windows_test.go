package process

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestEnvironmentVariableReadsChildProcess(t *testing.T) {
	cmd := exec.Command("ping", "-n", "5", "127.0.0.1")
	cmd.Env = append(os.Environ(), "PANEACEA_TEST_PANE=pane-42")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	defer cmd.Process.Kill()
	time.Sleep(200 * time.Millisecond)
	pid := uint32(cmd.Process.Pid)
	if value, ok := EnvironmentVariable(pid, "paneacea_test_pane"); !ok || value != "pane-42" {
		t.Fatalf("got %q ok=%v", value, ok)
	}
	if value, ok := EnvironmentVariable(pid, "PANEACEA_MISSING"); !ok || value != "" {
		t.Fatalf("missing variable: got %q ok=%v", value, ok)
	}
	if CachedEnvironmentVariable(pid, Generation(pid), "PANEACEA_TEST_PANE") != "pane-42" {
		t.Fatal("cached lookup failed")
	}
}

func TestLookupEnvironmentBlock(t *testing.T) {
	block, _ := windows.UTF16FromString("A=1")
	rest, _ := windows.UTF16FromString("Path=C:\\x")
	block = append(append(block, rest...), 0)
	if value, ok := lookupEnvironmentBlock(block, "PATH"); !ok || value != "C:\\x" {
		t.Fatalf("got %q ok=%v", value, ok)
	}
	if _, ok := lookupEnvironmentBlock(block, "PAT"); ok {
		t.Fatal("prefix must not match")
	}
}

func TestIsMSYSImage(t *testing.T) {
	for path, want := range map[string]bool{
		`C:\Program Files\Git\usr\bin\sh.exe`: true,
		`C:/msys64/usr/bin/bash.exe`:          true,
		`C:\Program Files\Git\bin\bash.exe`:   false,
		`C:\tools\paneacea-runtime.exe`:       false,
		"":                                    false,
	} {
		if got := IsMSYSImage(path); got != want {
			t.Fatalf("%s: got %v", path, got)
		}
	}
}

func TestImagePathOfCurrentProcess(t *testing.T) {
	exe, _ := os.Executable()
	if got := ImagePath(uint32(os.Getpid())); !strings.EqualFold(got, exe) {
		t.Fatalf("got %q want %q", got, exe)
	}
}

func TestOrphansReturnsProcessesWithDeadParents(t *testing.T) {
	items := []Info{{PID: 4}, {PID: 10, ParentPID: 4}, {PID: 11, ParentPID: 99}, {PID: 12, ParentPID: 11}}
	orphans := Orphans(items)
	if len(orphans) != 1 || orphans[0].PID != 11 {
		t.Fatalf("got %+v", orphans)
	}
}
