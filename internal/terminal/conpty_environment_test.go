//go:build windows

package terminal

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestEnvironmentBlockOmitsInheritedNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	block, err := environmentBlock(nil)
	if err != nil {
		t.Fatal(err)
	}
	value := string(utf16.Decode(block))
	if strings.Contains(value, "NO_COLOR=") {
		t.Fatal("inherited NO_COLOR was passed to the terminal")
	}
}

func TestEnvironmentBlockPreservesExplicitNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	block, err := environmentBlock(map[string]string{"NO_COLOR": "1"})
	if err != nil {
		t.Fatal(err)
	}
	value := string(utf16.Decode(block))
	if !strings.Contains(value, "NO_COLOR=1") {
		t.Fatal("explicit NO_COLOR was not passed to the terminal")
	}
}
