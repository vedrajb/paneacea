package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestHelpListsOpen(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "open") {
		t.Fatalf("help does not mention open:\n%s", out.String())
	}
}

func TestOpenLaunchesWithoutWaitingForTheApp(t *testing.T) {
	// The test binary has no paneacea.exe beside it, so open must fail fast instead of
	// connecting to or waiting on a runtime.
	t.Setenv("PANEACEA_PIPE", `\\.\pipe\paneacea-test-unavailable`)
	started := time.Now()
	err := Run([]string{"open"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "paneacea.exe") {
		t.Fatalf("open = %v, want missing paneacea.exe error", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("open took %v; it must not wait for the app", elapsed)
	}
}
