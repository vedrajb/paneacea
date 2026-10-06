package process

import (
	"os"
	"strings"
	"testing"
)

func TestCommandLineReadsCurrentProcess(t *testing.T) {
	commandLine := CommandLine(uint32(os.Getpid()))
	if !strings.Contains(strings.ToLower(commandLine), ".test") {
		t.Fatalf("unexpected command line %q", commandLine)
	}
}

func TestCachedCommandLineIsPrunedForDeadProcesses(t *testing.T) {
	pid := uint32(os.Getpid())
	first := CachedCommandLine(pid, Generation(pid))
	if first == "" || CachedCommandLine(pid, Generation(pid)) != first {
		t.Fatal("cached command line mismatch")
	}
	PruneCommandLines(nil)
	commandLineCache.Lock()
	_, ok := commandLineCache.items[pid]
	commandLineCache.Unlock()
	if ok {
		t.Fatal("expected cache entry to be pruned")
	}
}
