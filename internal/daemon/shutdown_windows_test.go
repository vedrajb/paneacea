package daemon

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/persistence"
)

func TestSystemShutdownPreservesSplitLayoutAcrossStartup(t *testing.T) {
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
	state := invoke(t, r, "workspace.create", Params{Name: "Reboot", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "cmd.exe", Arguments: []string{}}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	firstID := tab.ActivePaneID
	state = invoke(t, r, "pane.split", Params{PaneID: firstID, Orientation: "vertical", Executable: "cmd.exe", Arguments: []string{}}).(*model.State)
	_, tab = state.Tab(tab.ID)
	secondID := tab.ActivePaneID
	state = invoke(t, r, "pane.split", Params{PaneID: secondID, Orientation: "horizontal", Executable: "cmd.exe", Arguments: []string{}}).(*model.State)
	_, tab = state.Tab(tab.ID)
	invoke(t, r, "tab.rename", Params{TabID: tab.ID, Title: "Saved layout"})
	invoke(t, r, "pane.resize", Params{TabID: tab.ID, Ratio: 0.6})
	invoke(t, r, "pane.resize", Params{TabID: tab.ID, Path: []int{1}, Ratio: 0.4})
	state = invoke(t, r, "pane.focus", Params{PaneID: firstID}).(*model.State)

	exited := make(chan struct{}, len(state.Panes))
	r.mu.Lock()
	r.systemShuttingDown = func() bool {
		exited <- struct{}{}
		return true
	}
	r.mu.Unlock()
	for id := range state.Panes {
		invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "exit\r"})
	}
	for range state.Panes {
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
			t.Fatal("shell did not exit during simulated Windows shutdown")
		}
	}
	invoke(t, r, "state.get", nil)
	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Panes) != 3 || !reflect.DeepEqual(persisted.Workspaces, state.Workspaces) || persisted.ActiveWorkspaceID != state.ActiveWorkspaceID {
		t.Fatalf("shutdown changed the saved layout: %#v", persisted)
	}
	r.Close()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	restored := invoke(t, r, "state.get", nil).(*model.State)
	if len(restored.Panes) != 3 || !reflect.DeepEqual(restored.Workspaces, state.Workspaces) || restored.ActiveWorkspaceID != state.ActiveWorkspaceID {
		t.Fatalf("startup changed the saved layout: %#v", restored)
	}
	for id, pane := range state.Panes {
		current := restored.Panes[id]
		if current == nil || current.Status != "running" || current.PID == 0 || current.CurrentWorkingDirectory != pane.CurrentWorkingDirectory || current.Executable != pane.Executable || !reflect.DeepEqual(current.Arguments, pane.Arguments) {
			t.Fatalf("pane %s was not restored: %#v", id, current)
		}
	}
}
