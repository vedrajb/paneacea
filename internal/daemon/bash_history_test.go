package daemon

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/persistence"
)

func TestBashHistorySurvivesStartupOutput(t *testing.T) {
	var profile model.Profile
	for _, candidate := range Profiles() {
		if candidate.ID == "git-bash" && candidate.Available {
			profile = candidate
		}
	}
	if !profile.Available {
		t.Skip("Git Bash is not installed")
	}
	store, err := persistence.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	state := invoke(t, r, "workspace.create", Params{Name: "History", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: profile.Executable, Arguments: profile.Arguments, Environment: map[string]string{"HISTFILE": "/dev/null"}}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	id := tab.ActivePaneID
	waitFor := func(marker string) ipc.Output {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
			if bytes.Contains(output.Data, []byte(marker)) {
				return output
			}
			time.Sleep(25 * time.Millisecond)
		}
		output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
		t.Fatalf("shell did not produce %q: %q", marker, output.Data)
		return ipc.Output{}
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "printf 'SAVED_%s\\n' 'BEFORE_RESTART'\r"})
	waitFor("SAVED_BEFORE_RESTART")
	r.Close()
	r = nil
	history, err := store.LoadTerminalHistory(id)
	if err != nil || !bytes.Contains(history.Data, []byte("SAVED_BEFORE_RESTART")) {
		t.Fatalf("history was not saved: %v", err)
	}
	// Capture the earliest possible attach, before any new ConPTY output arrives.
	restored := newOutput()
	restored.emulator = vt.NewEmulator(history.Columns, history.Rows)
	restored.emulator.SetScrollbackSize(10000)
	if err = restored.restore(history.Data); err != nil {
		t.Fatal(err)
	}
	attached := restored.attachSnapshot(restored.scrollbackLines())
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "printf 'FRESH_%s\\n' 'AFTER_RESTART'\r"})
	waitFor("FRESH_AFTER_RESTART")
	invoke(t, r, "terminal.resize", Params{PaneID: id, Columns: 81, Rows: 45})
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "printf 'RESIZED_%s\\n' 'AFTER_RESTART'\r"})
	output := waitFor("RESIZED_AFTER_RESTART")
	r.mu.Lock()
	buffer := r.outputs[id]
	r.mu.Unlock()
	buffer.mu.Lock()
	startup := buffer.rangeBytes(attached.Sequence, buffer.end)
	buffer.mu.Unlock()
	if path := os.Getenv("PANEACEA_HISTORY_FIXTURE"); path != "" {
		data, err := json.Marshal(map[string]any{"attached": attached, "startup": startup})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Contains(output.Data, []byte("SAVED_BEFORE_RESTART")) {
		t.Fatalf("startup erased restored history; fresh output: %q", output.Data)
	}
}

func TestLoginBashTracksDirectoryAndRecallsCommandAfterRuntimeRestart(t *testing.T) {
	var profile model.Profile
	for _, candidate := range Profiles() {
		if candidate.ID == "git-bash" && candidate.Available {
			profile = candidate
		}
	}
	if !profile.Available {
		t.Skip("Git Bash is not installed")
	}
	root := t.TempDir()
	target := t.TempDir()
	bashPath := func(path string) string {
		return "/" + strings.ToLower(path[:1]) + strings.ReplaceAll(path[2:], "\\", "/")
	}
	store, err := persistence.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	state := invoke(t, r, "workspace.create", Params{Name: "Bash", RootDirectory: root}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: profile.Executable, Arguments: profile.Arguments}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	id := tab.ActivePaneID
	historyFile, err := store.BashHistoryPath(id)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "cd -- '" + bashPath(target) + "'\r"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state = invoke(t, r, "state.get", nil).(*model.State)
		if state.Panes[id].CurrentWorkingDirectory == target {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if state.Panes[id].CurrentWorkingDirectory != target {
		t.Fatalf("login Bash directory was not tracked: got %q, want %q", state.Panes[id].CurrentWorkingDirectory, target)
	}
	startupOutput := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
	if bytes.Contains(startupOutput.Data, []byte("__paneacea_prompt_hook")) || bytes.Contains(startupOutput.Data, []byte("PaneaceaSetup=")) {
		t.Fatalf("internal Bash setup command leaked into terminal output: %q", startupOutput.Data)
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "printf 'PANEACEA_RECALL_%s\\n' 'SAVED'\r"})
	deadline = time.Now().Add(5 * time.Second)
	saved := false
	for time.Now().Before(deadline) {
		output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
		data, _ := os.ReadFile(historyFile)
		if bytes.Contains(output.Data, []byte("PANEACEA_RECALL_SAVED")) && bytes.Contains(data, []byte("PANEACEA_RECALL_%s")) {
			saved = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !saved {
		t.Fatal("login Bash did not flush the command to its history file")
	}
	r.Close()
	r = nil
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	state = invoke(t, r, "state.get", nil).(*model.State)
	if state.Panes[id].CurrentWorkingDirectory != target {
		t.Fatalf("login Bash directory did not restore: got %q, want %q", state.Panes[id].CurrentWorkingDirectory, target)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		buffer := r.outputs[id]
		r.mu.Unlock()
		buffer.mu.Lock()
		ready := buffer.end > 0
		buffer.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	startupOutput = invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
	if bytes.Contains(startupOutput.Data, []byte("__paneacea_prompt_hook")) || bytes.Contains(startupOutput.Data, []byte("PaneaceaSetup=")) {
		t.Fatalf("internal Bash setup command leaked after runtime restart: %q", startupOutput.Data)
	}
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "\x1b[A\r"})
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
		if bytes.Count(output.Data, []byte("PANEACEA_RECALL_SAVED")) >= 2 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(historyFile)
	t.Fatalf("Up-arrow did not recall the command after the runtime restart; history file has command=%v size=%d", bytes.Contains(data, []byte("PANEACEA_RECALL_%s")), len(data))
}

func TestLoginBashKeepsCommandHistorySeparatePerPane(t *testing.T) {
	var profile model.Profile
	for _, candidate := range Profiles() {
		if candidate.ID == "git-bash" && candidate.Available {
			profile = candidate
		}
	}
	if !profile.Available {
		t.Skip("Git Bash is not installed")
	}
	store, err := persistence.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	state := invoke(t, r, "workspace.create", Params{Name: "Bash", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: profile.Executable, Arguments: profile.Arguments}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	firstID := tab.ActivePaneID
	state = invoke(t, r, "pane.split", Params{PaneID: firstID, Orientation: "vertical", Executable: profile.Executable, Arguments: profile.Arguments}).(*model.State)
	_, tab = state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	secondID := tab.ActivePaneID
	if firstID == secondID {
		t.Fatal("split did not create a second pane")
	}
	ids := []string{firstID, secondID}
	markers := []string{"PANEACEA_FIRST_SAVED", "PANEACEA_SECOND_SAVED"}
	commands := []string{"printf 'PANEACEA_FIRST_%s\\n' 'SAVED'\r", "printf 'PANEACEA_SECOND_%s\\n' 'SAVED'\r"}
	waitReady := func(id string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			r.mu.Lock()
			buffer := r.outputs[id]
			r.mu.Unlock()
			buffer.mu.Lock()
			ready := buffer.end > 0
			buffer.mu.Unlock()
			if ready {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("Bash pane %s did not become ready", id)
	}
	for i, id := range ids {
		waitReady(id)
		invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: commands[i]})
	}
	paths := make([]string, len(ids))
	for i, id := range ids {
		paths[i], err = store.BashHistoryPath(id)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			data, _ := os.ReadFile(paths[i])
			if bytes.Contains(data, []byte(commands[i][:len(commands[i])-1])) {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		data, err := os.ReadFile(paths[i])
		if err != nil || !bytes.Contains(data, []byte(commands[i][:len(commands[i])-1])) {
			t.Fatalf("pane %d command was not saved: %v", i, err)
		}
	}
	if paths[0] == paths[1] {
		t.Fatal("panes share a Bash history path")
	}
	for i := range ids {
		data, _ := os.ReadFile(paths[i])
		if bytes.Contains(data, []byte(commands[1-i][:len(commands[1-i])-1])) {
			t.Fatalf("pane %d history contains the other pane's command", i)
		}
	}
	r.Close()
	r = nil
	r, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		waitReady(id)
		invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: "\x1b[A\r"})
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
			if bytes.Count(output.Data, []byte(markers[i])) >= 2 {
				if bytes.Contains(output.Data, []byte(markers[1-i])) {
					t.Fatalf("pane %d recalled the other pane's command", i)
				}
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		output := invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output)
		if bytes.Count(output.Data, []byte(markers[i])) < 2 {
			t.Fatalf("pane %d did not recall its command after restart", i)
		}
	}
}
