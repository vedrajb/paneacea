package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu    sync.Mutex
	state *model.State
	fail  bool
}

func (s *memoryStore) Load() (*model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return model.NewState(), nil
	}
	return clone(s.state), nil
}
func (s *memoryStore) Save(state *model.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("test save failure")
	}
	s.state = clone(state)
	return nil
}
func invoke(t *testing.T, r *Runtime, method string, p any) any {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	value, err := r.Call(context.Background(), method, data)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return value
}
func TestWorkspacePersistenceAndMutationRollback(t *testing.T) {
	store := &memoryStore{}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	state := invoke(t, r, "workspace.create", Params{Name: "Project", RootDirectory: t.TempDir()}).(*model.State)
	id := state.ActiveWorkspaceID
	store.mu.Lock()
	store.fail = true
	store.mu.Unlock()
	data, _ := json.Marshal(Params{WorkspaceID: id, Name: "Uncommitted"})
	if _, err = r.Call(context.Background(), "workspace.rename", data); err == nil {
		t.Fatal("failed persistence returned success")
	}
	state = invoke(t, r, "state.get", nil).(*model.State)
	if state.Workspace(id).Name != "Project" {
		t.Fatal("failed mutation was not rolled back")
	}
	store.mu.Lock()
	store.fail = false
	store.mu.Unlock()
}
func TestDetachedTerminalAndRuntimeRestoration(t *testing.T) {
	store := &memoryStore{}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	state := invoke(t, r, "workspace.create", Params{Name: "Test", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile"}, Environment: map[string]string{"PANEACEA_TEST_ENV": "present"}}).(*model.State)
	w := state.Workspace(state.ActiveWorkspaceID)
	_, tab := state.Tab(w.ActiveTabID)
	id := tab.ActivePaneID
	pid := state.Panes[id].PID
	defer func() { r.Close() }()
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "Write-Output ('DETACHED_' + 'OK'); Write-Output $env:PANEACEA_TEST_ENV\r"})
	deadline := time.Now().Add(15 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		value := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
		if strings.Contains(string(value.Data), "DETACHED_OK") && strings.Contains(string(value.Data), "present") {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Fatal("headless shell did not produce output or receive environment")
	}
	invoke(t, r, "terminal.detach", Params{PaneID: id})
	state = invoke(t, r, "state.get", nil).(*model.State)
	if state.Panes[id].PID != pid || state.Panes[id].Status != "running" {
		t.Fatal("detach ended shell")
	}
	invoke(t, r, "terminal.resize", Params{PaneID: id, Columns: 90, Rows: 25})
	state = invoke(t, r, "pane.split", Params{PaneID: id, Orientation: "vertical"}).(*model.State)
	if len(state.Panes) != 2 {
		t.Fatal("split did not create second pane")
	}
	r.Close()
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	state = invoke(t, r, "state.get", nil).(*model.State)
	if len(state.Panes) != 2 || state.Panes[id].Status != "running" || state.Panes[id].PID == pid || state.Panes[id].Columns != 90 {
		t.Fatal("restart failed to restore pane launch and dimensions")
	}
	invoke(t, r, "workspace.close", Params{WorkspaceID: state.ActiveWorkspaceID})
	state = invoke(t, r, "state.get", nil).(*model.State)
	if len(state.Panes) != 0 || len(state.Workspaces) != 0 {
		t.Fatal("workspace ownership cleanup failed")
	}
}
func TestSplitPaneWorkingDirectoriesRestore(t *testing.T) {
	store := &memoryStore{}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { r.Close() }()
	var profile model.Profile
	for _, candidate := range Profiles() {
		if candidate.ID == "git-bash" && candidate.Available {
			profile = candidate
			break
		}
	}
	if profile.Executable == "" {
		t.Skip("Git Bash is not installed")
	}
	root := t.TempDir()
	other := t.TempDir()
	state := invoke(t, r, "workspace.create", Params{Name: "Test", RootDirectory: root}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: profile.Executable, Arguments: profile.Arguments}).(*model.State)
	w := state.Workspace(state.ActiveWorkspaceID)
	_, tab := state.Tab(w.ActiveTabID)
	firstID := tab.ActivePaneID
	state = invoke(t, r, "pane.split", Params{PaneID: firstID, Orientation: "vertical"}).(*model.State)
	_, tab = state.Tab(w.ActiveTabID)
	secondID := tab.ActivePaneID
	otherForBash := "/" + strings.ToLower(other[:1]) + strings.ReplaceAll(other[2:], "\\", "/")
	invoke(t, r, "pane.sendInput", Params{PaneID: secondID, Data: "cd -- '" + otherForBash + "'\r"})
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state = invoke(t, r, "state.get", nil).(*model.State)
		if state.Panes[secondID].CurrentWorkingDirectory == other {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if state.Panes[firstID].CurrentWorkingDirectory != root || state.Panes[secondID].CurrentWorkingDirectory != other {
		t.Fatalf("working directories were not tracked independently: first=%q second=%q", state.Panes[firstID].CurrentWorkingDirectory, state.Panes[secondID].CurrentWorkingDirectory)
	}
	r.Close()
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: firstID, Data: "pwd -W\r"})
	invoke(t, r, "pane.sendInput", Params{PaneID: secondID, Data: "pwd -W\r"})
	rootForBash := strings.ReplaceAll(root, "\\", "/")
	otherForBash = strings.ReplaceAll(other, "\\", "/")
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		first := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: firstID}).(ipc.Output)
		second := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: secondID}).(ipc.Output)
		if strings.Contains(string(first.Data), rootForBash) && strings.Contains(string(second.Data), otherForBash) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("restored panes did not start in their saved working directories")
}
func TestInvalidMethodsAndTargets(t *testing.T) {
	r, err := New(&memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, method := range []string{"unknown", "tab.focus", "pane.close", "agent.register", "workspace.rename", "pane.resize"} {
		if _, err = r.Call(context.Background(), method, json.RawMessage(`{}`)); err == nil {
			t.Fatalf("invalid request accepted: %s", method)
		}
	}
}
