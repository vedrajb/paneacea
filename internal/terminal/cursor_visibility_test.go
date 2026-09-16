//go:build windows

package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestConPTYForwardsHiddenCursor(t *testing.T) {
	output := make(chan string, 64)
	session, err := NewSession(Options{
		Executable: "powershell.exe",
		Arguments: []string{"-NoLogo", "-NoProfile", "-Command", "[Console]::Write(([char]27).ToString() + '[?25lCURSOR_HIDDEN'); Start-Sleep -Seconds 60"},
		WorkingDirectory: t.TempDir(),
		OnOutput: func(data string) { output <- data },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var collected strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case data := <-output:
			collected.WriteString(data)
			if strings.Contains(collected.String(), "CURSOR_HIDDEN") {
				if !strings.Contains(collected.String(), "\x1b[?25l") {
					t.Fatalf("ConPTY omitted cursor hide: %q", collected.String())
				}
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for output: %q", collected.String())
		}
	}
}
