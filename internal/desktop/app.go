package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/paneacea/paneacea/internal/ipc"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"sync"
)

type App struct {
	mu      sync.Mutex
	ctx     context.Context
	client  *ipc.Client
	streams map[string]*ipc.Client
}

func New() *App                            { return &App{streams: map[string]*ipc.Client{}} }
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	_, _ = a.Call("agent.restore", nil)
}
func (a *App) Shutdown(context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		_, _ = a.client.Call("agent.stop", nil)
		a.client.Close()
		a.client = nil
	}
	for _, c := range a.streams {
		c.Close()
	}
	a.streams = map[string]*ipc.Client{}
}
func (a *App) Call(method string, params json.RawMessage) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client == nil {
		client, err := ipc.Ensure(a.ctx)
		if err != nil {
			return nil, err
		}
		a.client = client
	}
	result, err := a.client.Call(method, params)
	if err != nil {
		a.client.Close()
		a.client = nil
	}
	return result, err
}
func (a *App) ReadOutput(streamID, id string, sequence uint64) (json.RawMessage, error) {
	a.mu.Lock()
	client := a.streams[streamID]
	method := "terminal.read"
	if client == nil {
		method = "terminal.attach"
		var err error
		client, err = ipc.Connect(a.ctx)
		if err != nil {
			a.mu.Unlock()
			return nil, err
		}
		a.streams[streamID] = client
	}
	a.mu.Unlock()
	result, err := client.Call(method, map[string]any{"paneId": id, "sequence": sequence})
	if err != nil {
		a.mu.Lock()
		if a.streams[streamID] == client {
			delete(a.streams, streamID)
		}
		a.mu.Unlock()
		client.Close()
	}
	return result, err
}
func (a *App) Detach(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if client := a.streams[id]; client != nil {
		client.Close()
		delete(a.streams, id)
	}
}
func (a *App) OpenFolder() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Workspace root folder"})
}
func (a *App) Copy(text string) error {
	if err := wailsruntime.ClipboardSetText(a.ctx, text); err != nil {
		return fmt.Errorf("could not copy terminal selection")
	}
	return nil
}
