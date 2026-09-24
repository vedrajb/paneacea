package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/persistence"
)

type unreadableHistoryStore struct {
	memoryStore
	mu             sync.Mutex
	saves          int
	deletes        int
	deleteAllCalls int
}

func (s *unreadableHistoryStore) LoadTerminalHistory(string) (*persistence.TerminalHistory, error) {
	return nil, errors.New("DPAPI key unavailable")
}

func (s *unreadableHistoryStore) SaveTerminalHistory(string, persistence.TerminalHistory) error {
	s.mu.Lock()
	s.saves++
	s.mu.Unlock()
	return nil
}

func (s *unreadableHistoryStore) DeleteTerminalHistory(string) error {
	s.mu.Lock()
	s.deletes++
	s.mu.Unlock()
	return nil
}

func (s *unreadableHistoryStore) DeleteAllTerminalHistory() error {
	s.mu.Lock()
	s.deleteAllCalls++
	s.mu.Unlock()
	return nil
}

func (s *unreadableHistoryStore) DeleteTerminalHistoryExcept([]string) error { return nil }

func TestUnreadableTerminalHistoryIsPreservedUntilItCanBeDecrypted(t *testing.T) {
	root := t.TempDir()
	workspaceID, tabID, paneID := model.ID(), model.ID(), model.ID()
	state := model.NewState()
	state.ActiveWorkspaceID = workspaceID
	state.Workspaces = []*model.Workspace{{ID: workspaceID, Name: "History", RootDirectory: root, ActiveTabID: tabID, Tabs: []*model.Tab{{ID: tabID, WorkspaceID: workspaceID, Title: "Terminal", TitleMode: "automatic", ActivePaneID: paneID, RootLayoutNode: &model.Layout{PaneID: paneID}}}}}
	state.Panes[paneID] = &model.Pane{ID: paneID, WorkspaceID: workspaceID, TabID: tabID, ProfileID: "cmd", Executable: "cmd.exe", Arguments: []string{"/Q"}, Environment: map[string]string{}, InitialWorkingDirectory: root, CurrentWorkingDirectory: root, Columns: 80, Rows: 24}
	store := &unreadableHistoryStore{memoryStore: memoryStore{state: state}}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	blocked := r.unreadableHistory[paneID]
	warning := r.state.Panes[paneID].Error
	r.mu.Unlock()
	if !blocked || !strings.Contains(warning, "DPAPI key unavailable") {
		r.Close()
		t.Fatalf("unreadable history was not retained as a pane warning: blocked=%v warning=%q", blocked, warning)
	}
	r.checkpointHistory(true)
	r.Close()
	store.mu.Lock()
	saves, deletes, deleteAllCalls := store.saves, store.deletes, store.deleteAllCalls
	store.mu.Unlock()
	if saves != 0 || deletes != 0 || deleteAllCalls != 0 {
		t.Fatalf("unreadable history was replaced or deleted: saves=%d deletes=%d deleteAll=%d", saves, deletes, deleteAllCalls)
	}
}

func TestTerminalHistoryRestoresIndependentlyAcrossSQLiteRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(store)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	defer func() {
		if r != nil {
			r.Close()
		}
		if store != nil {
			store.Close()
		}
	}()
	state := invoke(t, r, "workspace.create", Params{Name: "History", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile"}}).(*model.State)
	w := state.Workspace(state.ActiveWorkspaceID)
	_, tab := state.Tab(w.ActiveTabID)
	firstID := tab.ActivePaneID
	state = invoke(t, r, "pane.split", Params{PaneID: firstID, Orientation: "vertical", Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile"}}).(*model.State)
	_, tab = state.Tab(tab.ID)
	secondID := tab.ActivePaneID
	state = invoke(t, r, "pane.split", Params{PaneID: secondID, Orientation: "horizontal", Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile"}}).(*model.State)
	_, tab = state.Tab(tab.ID)
	paneIDs := tab.RootLayoutNode.Leaves()
	markers := map[string]string{paneIDs[0]: "PANE_HISTORY_ONE_Ω", paneIDs[1]: "PANE_HISTORY_TWO_Ω", paneIDs[2]: "PANE_HISTORY_THREE_Ω"}
	for id, marker := range markers {
		invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "Write-Output '" + marker + "'\r"})
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		allVisible := true
		for id, marker := range markers {
			output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
			if !bytes.Contains(output.Data, []byte(marker)) {
				allVisible = false
				break
			}
		}
		if allVisible {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for id, marker := range markers {
		output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
		if !bytes.Contains(output.Data, []byte(marker)) {
			t.Fatalf("pane %s did not display its history marker %q", id, marker)
		}
	}
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		allSaved := true
		for id, marker := range markers {
			history, loadErr := store.LoadTerminalHistory(id)
			if loadErr != nil || !bytes.Contains(history.Data, []byte(marker)) {
				allSaved = false
				break
			}
		}
		if allSaved {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for id, marker := range markers {
		history, loadErr := store.LoadTerminalHistory(id)
		if loadErr != nil || !bytes.Contains(history.Data, []byte(marker)) {
			t.Fatalf("periodic checkpoint missed pane %s history: err=%v", id, loadErr)
		}
	}
	wantWorkspace := clone(state).Workspaces
	wantActive := state.ActiveWorkspaceID
	r.Close()
	r = nil
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	store, err = persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err = New(store)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	state = invoke(t, r, "state.get", nil).(*model.State)
	if state.ActiveWorkspaceID != wantActive || len(state.Workspaces) != len(wantWorkspace) {
		t.Fatal("history restore changed the saved pane layout")
	}
	for i, workspace := range state.Workspaces {
		if workspace.ID != wantWorkspace[i].ID || workspace.ActiveTabID != wantWorkspace[i].ActiveTabID || len(workspace.Tabs) != len(wantWorkspace[i].Tabs) {
			t.Fatal("history restore changed the saved workspace or tab order")
		}
		for j, restoredTab := range workspace.Tabs {
			if restoredTab.ID != wantWorkspace[i].Tabs[j].ID || restoredTab.ActivePaneID != wantWorkspace[i].Tabs[j].ActivePaneID || !reflect.DeepEqual(restoredTab.RootLayoutNode, wantWorkspace[i].Tabs[j].RootLayoutNode) {
				t.Fatal("history restore changed the saved pane split tree")
			}
		}
	}
	restoredCounts := map[string]int{}
	for id, marker := range markers {
		output := invoke(t, r, "terminal.attach", Params{PaneID: id}).(ipc.Output)
		if !output.Restored || !bytes.Contains(output.Data, []byte(marker)) {
			t.Fatalf("pane %s history was not restored: restored=%v", id, output.Restored)
		}
		restoredCounts[id] = bytes.Count(output.Data, []byte(marker))
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: paneIDs[0], Data: "Write-Output 'PANE_HISTORY_FRESH'\r"})
	deadline = time.Now().Add(15 * time.Second)
	freshSeen := false
	for time.Now().Before(deadline) {
		output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: paneIDs[0]}).(ipc.Output)
		if bytes.Contains(output.Data, []byte("PANE_HISTORY_FRESH")) {
			if bytes.Count(output.Data, []byte(markers[paneIDs[0]])) != restoredCounts[paneIDs[0]] {
				t.Fatal("restored output was duplicated after fresh shell output")
			}
			freshSeen = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !freshSeen {
		t.Fatal("new shell output was not appended after restored history")
	}
	invoke(t, r, "pane.close", Params{PaneID: paneIDs[0]})
	if _, err = store.LoadTerminalHistory(paneIDs[0]); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("closing a pane retained its saved history")
	}
	state = invoke(t, r, "state.get", nil).(*model.State)
	state.Settings.TerminalHistoryLines = 0
	invoke(t, r, "settings.set", Params{Settings: &state.Settings})
	for _, id := range paneIDs[1:] {
		if _, err = store.LoadTerminalHistory(id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("Off retained saved history for pane %s", id)
		}
	}
}

func TestViewportOffsetSurvivesAttachWithinRuntimeOnly(t *testing.T) {
	store := &memoryStore{}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	output := newOutput()
	output.emulator = vt.NewEmulator(20, 2)
	output.append([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive\r\nsix\r\n"))
	r.mu.Lock()
	r.outputs["pane"] = output
	r.mu.Unlock()
	maxOffset := output.scrollbackLines()
	if maxOffset == 0 {
		r.Close()
		t.Fatal("test output did not create scrollback")
	}
	wantOffset := min(2, maxOffset)
	invoke(t, r, "terminal.viewport.set", Params{PaneID: "pane", Offset: wantOffset})
	attached := invoke(t, r, "terminal.attach", Params{PaneID: "pane"}).(ipc.Output)
	if attached.ViewportOffset != wantOffset {
		t.Fatalf("attached viewport offset = %d, want %d", attached.ViewportOffset, wantOffset)
	}
	if _, err = r.Call(context.Background(), "terminal.viewport.set", json.RawMessage(`{"paneId":"pane","offset":-1}`)); err == nil {
		t.Fatal("negative viewport offset was accepted")
	}
	r.Close()
	r = nil
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	output = newOutput()
	output.emulator = vt.NewEmulator(20, 2)
	r.mu.Lock()
	r.outputs["pane"] = output
	r.mu.Unlock()
	attached = invoke(t, r, "terminal.attach", Params{PaneID: "pane"}).(ipc.Output)
	if attached.ViewportOffset != 0 {
		t.Fatalf("viewport offset survived runtime restart: %d", attached.ViewportOffset)
	}
	r.Close()
}
