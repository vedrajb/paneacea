package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/paneacea/paneacea/internal/daemon"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/persistence"
)

var errHostClosed = errors.New("runtime unavailable: Paneacea is shutting down")

// host runs the terminal runtime inside the desktop process and serves it on the named pipe
// for paneacea-cli and pca. It lives exactly as long as the window.
type host struct {
	once    sync.Once
	mu      sync.Mutex
	closed  bool
	ready   chan struct{}
	err     error
	runtime *daemon.Runtime
	store   *persistence.Store
	cancel  context.CancelFunc
	served  chan struct{}
}

func newHost() *host { return &host{ready: make(chan struct{})} }

// dataDirectory is PANEACEA_DATA_DIR, or the directory of the executable.
func dataDirectory() (string, error) {
	if directory := os.Getenv("PANEACEA_DATA_DIR"); directory != "" {
		return directory, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}

// begin starts the runtime in the background so the window can appear while panes launch.
func (h *host) begin() {
	h.once.Do(func() { go h.start() })
}

func (h *host) start() {
	defer close(h.ready)
	// The pipe doubles as the lock on the database: only one runtime per install folder.
	listener, err := ipc.Listen()
	if err != nil {
		h.err = fmt.Errorf("runtime unavailable: another Paneacea runtime is already running for this folder (stop any old paneacea-runtime.exe): %w", err)
		return
	}
	directory, err := dataDirectory()
	if err == nil {
		err = os.MkdirAll(directory, 0o700)
	}
	var store *persistence.Store
	if err == nil {
		store, err = persistence.Open(filepath.Join(directory, "paneacea.db"))
	}
	var runtime *daemon.Runtime
	if err == nil {
		if runtime, err = daemon.New(store); err != nil {
			store.Close()
		}
	}
	if err != nil {
		listener.Close()
		h.err = fmt.Errorf("runtime unavailable: %w", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.runtime, h.store, h.cancel, h.served = runtime, store, cancel, make(chan struct{})
	go func() {
		defer close(h.served)
		if err := daemon.Serve(ctx, listener, runtime); err != nil {
			log.Printf("serve paneacea pipe: %v", err)
		}
	}()
}

func (h *host) wait() (*daemon.Runtime, error) {
	<-h.ready
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, errHostClosed
	}
	return h.runtime, h.err
}

// close stops agents, closes the pipe and shuts the runtime down, saving layout and history.
func (h *host) close() {
	h.once.Do(func() {
		h.err = errHostClosed
		close(h.ready)
	})
	<-h.ready
	h.mu.Lock()
	if h.closed || h.runtime == nil {
		h.closed = true
		h.mu.Unlock()
		return
	}
	h.closed = true
	runtime := h.runtime
	h.mu.Unlock()
	_, _ = runtime.Call(context.Background(), "agent.stop", nil)
	h.cancel()
	<-h.served
	runtime.Close()
	if err := h.store.Close(); err != nil {
		log.Printf("close paneacea database: %v", err)
	}
}
