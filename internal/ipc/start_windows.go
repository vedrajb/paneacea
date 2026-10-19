package ipc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// appStartTimeout covers the window opening and saved panes launching before the pipe is served.
const appStartTimeout = 15 * time.Second

// Launch starts paneacea.exe from this executable's folder, detached from the caller's console,
// and returns without waiting. If Paneacea is already running, the new process focuses it and exits.
func Launch() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	directory := filepath.Dir(executable)
	path := filepath.Join(directory, "paneacea.exe")
	if _, err = os.Stat(path); err != nil {
		return fmt.Errorf("build paneacea.exe alongside the CLI: %w", err)
	}
	cmd := exec.Command(path)
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
	if err = cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Ensure connects to the running Paneacea app, starting paneacea.exe in the background if it is
// not running, so CLI callers never block on the window.
func Ensure(ctx context.Context) (*Client, error) {
	client, err := Connect(ctx)
	if err == nil {
		return client, nil
	}
	if address := os.Getenv("PANEACEA_PIPE"); address != "" {
		return nil, fmt.Errorf("runtime unavailable at PANEACEA_PIPE %q: %w", address, err)
	}
	if err = Launch(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(appStartTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		if client, err := Connect(ctx); err == nil {
			return client, nil
		}
	}
	return nil, fmt.Errorf("Paneacea did not become ready")
}
