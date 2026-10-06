package daemon

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/persistence"
)

func TestRestartRestoresWorkingDirectoryWithoutReplayingOutput(t *testing.T) {
	executable, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip("cmd.exe is not installed")
	}
	const marker = "OLD_SESSION_OUTPUT_MARKER"
	seed := newOutput()
	seed.emulator = vt.NewEmulator(80, 24)
	seed.emulator.SetScrollbackSize(100)
	seed.append([]byte(marker + "\r\n"))
	data, columns, rows, _, err := seed.persistedSnapshot(2000, persistence.MaxTerminalHistoryBytes)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	cwd := t.TempDir()
	workspaceID, tabID, paneID := model.ID(), model.ID(), model.ID()
	state := model.NewState()
	state.ActiveWorkspaceID = workspaceID
	state.Workspaces = []*model.Workspace{{ID: workspaceID, Name: "History", RootDirectory: root, ActiveTabID: tabID, Tabs: []*model.Tab{{ID: tabID, WorkspaceID: workspaceID, Title: "Terminal", TitleMode: "automatic", ActivePaneID: paneID, RootLayoutNode: &model.Layout{PaneID: paneID}}}}}
	state.Panes[paneID] = &model.Pane{ID: paneID, WorkspaceID: workspaceID, TabID: tabID, ProfileID: "cmd", Executable: executable, Arguments: []string{"/Q"}, Environment: map[string]string{}, InitialWorkingDirectory: root, CurrentWorkingDirectory: cwd, Columns: 400, Rows: 24}
	store := &restoredViewportStore{memoryStore: memoryStore{state: state}, history: persistence.TerminalHistory{Version: 1, Columns: columns, Rows: rows, Data: data}}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	output := invoke(t, r, "terminal.attach", Params{PaneID: paneID}).(ipc.Output)
	if output.Restored || bytes.Contains(output.Data, []byte(marker)) {
		t.Fatalf("saved terminal output was replayed: restored=%v", output.Restored)
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: paneID, Data: "cd\r"})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		output = invoke(t, r, "terminal.requestSnapshot", Params{PaneID: paneID}).(ipc.Output)
		if strings.Contains(strings.ToLower(string(output.Data)), strings.ToLower(cwd)) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("shell did not start in the saved working directory %q; output: %q", cwd, output.Data)
}
