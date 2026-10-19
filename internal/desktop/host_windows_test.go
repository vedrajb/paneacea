package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
)

func startTestApp(t *testing.T) *App {
	t.Helper()
	app := New()
	app.host.begin()
	if _, err := app.host.wait(); err != nil {
		t.Fatal(err)
	}
	return app
}

func useTestData(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("PANEACEA_DATA_DIR", directory)
	t.Setenv("PANEACEA_PIPE", "")
	return directory
}

func callState(t *testing.T, app *App, method string, params any) *model.State {
	t.Helper()
	var raw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		raw = data
	}
	result, err := app.Call(method, raw)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var state model.State
	if err = json.Unmarshal(result, &state); err != nil {
		t.Fatal(err)
	}
	return &state
}

func createCmdPane(t *testing.T, app *App, root string) (*model.State, string) {
	t.Helper()
	state := callState(t, app, "workspace.create", map[string]any{"name": "Test", "rootDirectory": root})
	state = callState(t, app, "tab.create", map[string]any{"workspaceId": state.ActiveWorkspaceID, "executable": "cmd.exe", "arguments": []string{}})
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	return state, tab.ActivePaneID
}

func TestDataDirectoryPrefersEnvironment(t *testing.T) {
	t.Setenv("PANEACEA_DATA_DIR", `C:\paneacea-data`)
	if directory, err := dataDirectory(); err != nil || directory != `C:\paneacea-data` {
		t.Fatalf("dataDirectory = %q, %v", directory, err)
	}
	t.Setenv("PANEACEA_DATA_DIR", "")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if directory, err := dataDirectory(); err != nil || directory != filepath.Dir(executable) {
		t.Fatalf("default dataDirectory = %q, %v; want %q", directory, err, filepath.Dir(executable))
	}
}

func TestAppHostsRuntimeInProcessAndOnPipe(t *testing.T) {
	data := useTestData(t)
	app := startTestApp(t)
	callState(t, app, "state.get", nil)
	client, err := ipc.Connect(context.Background())
	if err != nil {
		app.Shutdown(context.Background())
		t.Fatalf("CLI pipe unavailable: %v", err)
	}
	if _, err = client.Call("state.get", nil); err != nil {
		t.Fatalf("pipe state.get: %v", err)
	}
	client.Close()
	app.Shutdown(context.Background())
	if _, err = os.Stat(filepath.Join(data, "paneacea.db")); err != nil {
		t.Fatalf("database not created in PANEACEA_DATA_DIR: %v", err)
	}
	if client, err = ipc.Connect(context.Background()); err == nil {
		client.Close()
		t.Fatal("pipe still served after the window shut down")
	}
	if _, err = app.Call("state.get", nil); err == nil || !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("Call after shutdown = %v, want shutting down error", err)
	}
}

func TestSecondHostDoesNotOpenDatabase(t *testing.T) {
	useTestData(t)
	first := startTestApp(t)
	defer first.Shutdown(context.Background())
	second := newHost()
	second.begin()
	if _, err := second.wait(); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second host = %v, want already running error", err)
	}
	second.close()
	callState(t, first, "state.get", nil)
}

func TestShutdownBeforeStartIsSafe(t *testing.T) {
	app := New()
	app.Shutdown(context.Background())
	if _, err := app.Call("state.get", nil); err == nil {
		t.Fatal("Call succeeded on a host that was never started")
	}
}

func TestDetachCancelsPendingRead(t *testing.T) {
	useTestData(t)
	app := startTestApp(t)
	defer app.Shutdown(context.Background())
	_, paneID := createCmdPane(t, app, t.TempDir())
	if _, err := app.ReadOutput("view", paneID, 0); err != nil {
		t.Fatalf("attach: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		// A sequence past the end waits for output that never arrives, like an idle terminal.
		_, err := app.ReadOutput("view", paneID, 1<<62)
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	app.Detach("view")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("detached read returned no error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Detach did not end the pending read")
	}
}

func TestShutdownKeepsPanesForNextLaunch(t *testing.T) {
	useTestData(t)
	root := t.TempDir()
	app := startTestApp(t)
	_, paneID := createCmdPane(t, app, root)
	app.Shutdown(context.Background())

	next := startTestApp(t)
	defer next.Shutdown(context.Background())
	state := callState(t, next, "state.get", nil)
	pane := state.Panes[paneID]
	if pane == nil || pane.Status != "running" || pane.PID == 0 || pane.Executable != "cmd.exe" {
		t.Fatalf("pane was not relaunched after restart: %#v", pane)
	}
}
