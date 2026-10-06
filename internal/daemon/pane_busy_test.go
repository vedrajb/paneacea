package daemon

import (
	"testing"
	"time"

	"github.com/paneacea/paneacea/internal/model"
)

// waitForBusy polls the pane's Busy flag until it equals want.
func waitForBusy(t *testing.T, r *Runtime, id string, want bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		r.mu.Lock()
		busy, aware := r.state.Panes[id].Busy, r.promptAware[id]
		r.mu.Unlock()
		if aware && busy == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("busy=%v promptAware=%v, want busy=%v", busy, aware, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func checkBackgroundJobsAreNotBusy(t *testing.T, executable string, arguments []string, foreground, background string) {
	r, err := New(&memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	state := invoke(t, r, "workspace.create", Params{Name: "Test", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: executable, Arguments: arguments}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	id := tab.ActivePaneID
	// The first prompt makes the pane prompt-aware and idle.
	waitForBusy(t, r, id, false)

	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: foreground + "\r"})
	waitForBusy(t, r, id, true)
	waitForBusy(t, r, id, false)

	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: background + "\r"})
	waitForBusy(t, r, id, false)
	// Stays idle across monitor ticks while the background job runs.
	time.Sleep(4500 * time.Millisecond)
	waitForBusy(t, r, id, false)
}

func TestPowerShellBackgroundJobsDoNotMakePaneBusy(t *testing.T) {
	checkBackgroundJobsAreNotBusy(t, "powershell.exe", []string{"-NoLogo", "-NoProfile"},
		"Start-Sleep -Seconds 2", "$null = Start-Job { Start-Sleep -Seconds 30 }")
}

func TestBashBackgroundJobsDoNotMakePaneBusy(t *testing.T) {
	var profile model.Profile
	for _, candidate := range Profiles() {
		if candidate.ID == "git-bash" && candidate.Available {
			profile = candidate
		}
	}
	if !profile.Available {
		t.Skip("Git Bash is not installed")
	}
	checkBackgroundJobsAreNotBusy(t, profile.Executable, profile.Arguments, "sleep 2", "sleep 30 &")
}
