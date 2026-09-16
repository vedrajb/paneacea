package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDataDirectoryUsesRuntimeDirectory(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := runtimeDataDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if data != filepath.Dir(executable) {
		t.Fatalf("default data directory = %q, want %q", data, filepath.Dir(executable))
	}
}
