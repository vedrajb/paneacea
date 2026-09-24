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

func Ensure(ctx context.Context) (*Client, error) {
	client, err := Connect(ctx)
	if err == nil {
		return client, nil
	}
	if address := os.Getenv("PANEACEA_PIPE"); address != "" {
		return nil, fmt.Errorf("runtime unavailable at PANEACEA_PIPE %q: %w", address, err)
	}
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path = filepath.Join(filepath.Dir(path), "paneacea-runtime.exe")
	if _, err = os.Stat(path); err != nil {
		return nil, fmt.Errorf("build paneacea-runtime.exe alongside the GUI: %w", err)
	}
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x00000200}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()
	for i := 0; i < 50; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		if client, err := Connect(ctx); err == nil {
			return client, nil
		}
	}
	return nil, fmt.Errorf("runtime did not become ready")
}
func EnsureLocal(ctx context.Context) (*Client, error) {
	client, err := ConnectLocal(ctx)
	if err == nil {
		return client, nil
	}
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path = filepath.Join(filepath.Dir(path), "paneacea-runtime.exe")
	if _, err = os.Stat(path); err != nil {
		return nil, fmt.Errorf("build paneacea-runtime.exe alongside the GUI: %w", err)
	}
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x00000200}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()
	for i := 0; i < 50; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		if client, err := ConnectLocal(ctx); err == nil {
			return client, nil
		}
	}
	return nil, fmt.Errorf("runtime did not become ready")
}
