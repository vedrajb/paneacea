package ipc

import (
	"context"
	"strings"
	"testing"
)

func TestEnsureDoesNotStartLocalRuntimeForPipeOverride(t *testing.T) {
	const address = `\\.\pipe\paneacea-unavailable`
	t.Setenv("PANEACEA_PIPE", address)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Ensure(ctx)
	if err == nil {
		t.Fatal("Ensure returned a client")
	}
	if !strings.Contains(err.Error(), "runtime unavailable") {
		t.Fatalf("Ensure error = %q, want runtime unavailable", err)
	}
	if !strings.Contains(err.Error(), "PANEACEA_PIPE") {
		t.Fatalf("Ensure error = %q, want PANEACEA_PIPE", err)
	}
}
