//go:build windows

package persistence

import (
	"bytes"
	"testing"
)

func TestDPAPIProtectsAndRestoresTerminalHistory(t *testing.T) {
	protector := systemHistoryProtector{}
	plain := []byte("terminal history is private")
	encrypted, err := protector.Protect(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("DPAPI result contains the plaintext history")
	}
	restored, err := protector.Unprotect(encrypted)
	if err != nil || !bytes.Equal(restored, plain) {
		t.Fatalf("DPAPI round trip failed: %q, %v", restored, err)
	}
	if _, err = protector.Unprotect([]byte("corrupt")); err == nil {
		t.Fatal("corrupt history was accepted")
	}
}
