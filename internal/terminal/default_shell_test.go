//go:build windows

package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultShellPrefersGitBash(t *testing.T) {
	gitBash := filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe")
	if _, err := exec.LookPath(gitBash); err != nil {
		t.Skip("Git Bash is not installed")
	}
	executable, arguments := defaultShell()
	if !strings.EqualFold(filepath.Base(executable), "bash.exe") {
		t.Fatalf("default shell = %q, want Git Bash", executable)
	}
	if len(arguments) != 2 || arguments[0] != "--login" || arguments[1] != "-i" {
		t.Fatalf("Git Bash arguments = %#v, want [--login -i]", arguments)
	}
}
