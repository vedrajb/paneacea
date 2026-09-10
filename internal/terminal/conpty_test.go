//go:build windows

package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestPowerShellConPTYRoundTripAndResize(t *testing.T) {
	output := make(chan string, 16)
	session, err := NewSession(Options{
		BatchInterval: 2 * time.Millisecond,
		BatchBytes:    1024,
		OnOutput: func(data string) {
			output <- data
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if err := session.Resize(100, 32); err != nil {
		t.Fatalf("resize ConPTY session: %v", err)
	}
	if err := session.Write([]byte("Write-Output \"PANEACEA-CONPTY-OK\"\r")); err != nil {
		t.Fatalf("write ConPTY input: %v", err)
	}

	var collected strings.Builder
	deadline := time.After(10 * time.Second)
	for !strings.Contains(collected.String(), "PANEACEA-CONPTY-OK") {
		select {
		case chunk := <-output:
			collected.WriteString(chunk)
		case <-deadline:
			t.Fatalf("timed out waiting for PowerShell output: %q", collected.String())
		}
	}
}
