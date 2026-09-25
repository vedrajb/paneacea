package daemon

import (
	"os/exec"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/persistence"
)

type restoredViewportStore struct {
	memoryStore
	history persistence.TerminalHistory
}

func (s *restoredViewportStore) SaveTerminalHistory(_ string, history persistence.TerminalHistory) error {
	s.mu.Lock()
	s.history = history
	s.mu.Unlock()
	return nil
}

func (s *restoredViewportStore) LoadTerminalHistory(_ string) (*persistence.TerminalHistory, error) {
	s.mu.Lock()
	history := s.history
	history.Data = append([]byte(nil), history.Data...)
	s.mu.Unlock()
	return &history, nil
}

func (s *restoredViewportStore) DeleteTerminalHistory(string) error { return nil }
func (s *restoredViewportStore) DeleteAllTerminalHistory() error    { return nil }
func (s *restoredViewportStore) DeleteTerminalHistoryExcept([]string) error {
	return nil
}

func TestRestoredViewportOpensAtBottom(t *testing.T) {
	executable, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip("cmd.exe is not installed")
	}
	seed := newOutput()
	seed.emulator = vt.NewEmulator(20, 2)
	seed.emulator.SetScrollbackSize(100)
	seed.append([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive\r\nsix\r\n"))
	data, columns, rows, _, err := seed.persistedSnapshot(2000, persistence.MaxTerminalHistoryBytes)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	workspaceID, tabID, paneID := model.ID(), model.ID(), model.ID()
	state := model.NewState()
	state.ActiveWorkspaceID = workspaceID
	state.Workspaces = []*model.Workspace{{ID: workspaceID, Name: "History", RootDirectory: root, ActiveTabID: tabID, Tabs: []*model.Tab{{ID: tabID, WorkspaceID: workspaceID, Title: "Terminal", TitleMode: "automatic", ActivePaneID: paneID, RootLayoutNode: &model.Layout{PaneID: paneID}}}}}
	state.Panes[paneID] = &model.Pane{ID: paneID, WorkspaceID: workspaceID, TabID: tabID, ProfileID: "cmd", Executable: executable, Arguments: []string{"/Q"}, Environment: map[string]string{}, InitialWorkingDirectory: root, CurrentWorkingDirectory: root, Columns: uint16(columns), Rows: uint16(rows)}
	store := &restoredViewportStore{memoryStore: memoryStore{state: state}, history: persistence.TerminalHistory{Version: 1, Columns: columns, Rows: rows, Data: data}}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	output := invoke(t, r, "terminal.attach", Params{PaneID: paneID}).(ipc.Output)
	if !output.Restored {
		t.Fatal("restored history was not marked as restored")
	}
	if output.ViewportOffset != 0 {
		t.Fatalf("restored viewport offset = %d, want 0", output.ViewportOffset)
	}
}
