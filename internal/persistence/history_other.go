//go:build !windows

package persistence

import "fmt"

type systemHistoryProtector struct{}

func (systemHistoryProtector) Protect([]byte) ([]byte, error) {
	return nil, fmt.Errorf("terminal history protection requires Windows DPAPI")
}

func (systemHistoryProtector) Unprotect([]byte) ([]byte, error) {
	return nil, fmt.Errorf("terminal history protection requires Windows DPAPI")
}
