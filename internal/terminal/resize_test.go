package terminal

import "testing"

func TestValidateSize(t *testing.T) {
	if err := ValidateSize(80, 24); err != nil {
		t.Fatalf("valid terminal size rejected: %v", err)
	}
	if err := ValidateSize(1, 24); err == nil {
		t.Fatal("one-column terminal size was accepted")
	}
	if err := ValidateSize(80, 0); err == nil {
		t.Fatal("zero-row terminal size was accepted")
	}
}
