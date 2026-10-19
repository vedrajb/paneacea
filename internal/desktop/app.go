package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// stream is one terminal view's output subscription; cancelling it ends a pending read.
type stream struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type App struct {
	mu      sync.Mutex
	ctx     context.Context
	host    *host
	streams map[string]*stream
}

func New() *App { return &App{host: newHost(), streams: map[string]*stream{}} }

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.host.begin()
}
func (a *App) DomReady(ctx context.Context) {
	wailsruntime.WindowShow(ctx)
	found := focusWebView()
	for attempt := 0; !found && attempt < 20; attempt++ {
		time.Sleep(100 * time.Millisecond)
		found = focusWebView()
	}
}

// Shutdown stops agents and shells and saves state; nothing keeps running after the window.
func (a *App) Shutdown(context.Context) {
	a.mu.Lock()
	for _, s := range a.streams {
		s.cancel()
	}
	a.streams = map[string]*stream{}
	a.mu.Unlock()
	a.host.close()
}

// SecondInstance brings the existing window forward when Paneacea is launched again.
func (a *App) SecondInstance(options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowShow(a.ctx)
	focusWebView()
}

func (a *App) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	runtime, err := a.host.wait()
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	switch value := params.(type) {
	case json.RawMessage:
		raw = value
	case nil:
	default:
		if raw, err = json.Marshal(value); err != nil {
			return nil, err
		}
	}
	result, err := runtime.Call(ctx, method, raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (a *App) Call(method string, params json.RawMessage) (json.RawMessage, error) {
	return a.call(context.Background(), method, params)
}
func (a *App) ReadOutput(streamID, id string, sequence uint64) (json.RawMessage, error) {
	a.mu.Lock()
	current := a.streams[streamID]
	method := "terminal.read"
	if current == nil {
		method = "terminal.attach"
		ctx, cancel := context.WithCancel(context.Background())
		current = &stream{ctx: ctx, cancel: cancel}
		a.streams[streamID] = current
	}
	a.mu.Unlock()
	result, err := a.call(current.ctx, method, map[string]any{"paneId": id, "sequence": sequence})
	if err == nil && current.ctx.Err() != nil {
		err = current.ctx.Err()
	}
	if err != nil {
		a.mu.Lock()
		if a.streams[streamID] == current {
			delete(a.streams, streamID)
		}
		a.mu.Unlock()
		current.cancel()
	}
	return result, err
}
func (a *App) Detach(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if current := a.streams[id]; current != nil {
		current.cancel()
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
