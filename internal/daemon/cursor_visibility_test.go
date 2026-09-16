package daemon

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/paneacea/paneacea/internal/model"
	"strings"
	"testing"
)

func TestSnapshotPreservesCursorVisibility(t *testing.T) {
	r, err := New(&memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	state := invoke(t, r, "workspace.create", Params{Name: "Cursor", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile", "-Command", "Start-Sleep -Seconds 60"}}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	output := r.outputs[tab.ActivePaneID]
	output.append([]byte(ansi.HideCursor))
	if !strings.Contains(string(output.snapshot().Data), ansi.HideCursor) {
		t.Fatal("snapshot made the hidden cursor visible")
	}
	output.append([]byte(ansi.ShowCursor))
	if !strings.Contains(string(output.snapshot().Data), ansi.ShowCursor) {
		t.Fatal("snapshot did not restore cursor visibility")
	}
}
