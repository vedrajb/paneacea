package ipc

import (
	"strings"
	"testing"
)

func TestAddressUsesInstanceID(t *testing.T) {
	id, err := InstanceID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "S-1-") || strings.ContainsAny(id, `\/`) {
		t.Fatalf("InstanceID = %q, want SID-hash without path separators", id)
	}
	address, err := Address()
	if err != nil {
		t.Fatal(err)
	}
	if address != `\\.\pipe\paneacea-go-`+id {
		t.Fatalf("Address = %q, want pipe named from InstanceID %q", address, id)
	}
}
