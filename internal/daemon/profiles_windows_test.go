//go:build windows

package daemon

import (
	"testing"
)

func TestProfilesPrioritizeGitBashThenPowerShellAndCmd(t *testing.T) {
	profiles := Profiles()
	expected := []string{"git-bash", "pwsh", "powershell", "cmd"}
	if len(profiles) < len(expected) {
		t.Fatalf("profiles = %d, want at least %d", len(profiles), len(expected))
	}
	for index, id := range expected {
		if profiles[index].ID != id {
			t.Fatalf("profile %d = %q, want %q", index, profiles[index].ID, id)
		}
	}
}
