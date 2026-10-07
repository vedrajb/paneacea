package terminal

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// stuckBackend models a console host that never releases its pipes: reads and waits only end
// when the test releases them, regardless of Close.
type stuckBackend struct {
	release     chan struct{}
	closeOnce   sync.Once
	exitOnClose bool
}

func newStuckBackend(exitOnClose bool) *stuckBackend {
	return &stuckBackend{release: make(chan struct{}), exitOnClose: exitOnClose}
}

func (b *stuckBackend) Read([]byte) (int, error)    { <-b.release; return 0, io.EOF }
func (b *stuckBackend) Write(p []byte) (int, error) { return len(p), nil }
func (b *stuckBackend) Resize(uint16, uint16) error { return nil }
func (b *stuckBackend) Wait() error                 { <-b.release; return nil }
func (b *stuckBackend) PID() uint32                 { return 1 }
func (b *stuckBackend) Close() error {
	if b.exitOnClose {
		b.closeOnce.Do(func() { close(b.release) })
	}
	return nil
}

func startTestSession(backend backend) *Session {
	session := &Session{backend: backend, done: make(chan struct{}), readDone: make(chan struct{})}
	session.batcher = NewOutputBatcher(time.Millisecond, 1024, nil)
	go session.readOutput()
	go session.waitForExit()
	return session
}

func withCloseTimeout(t *testing.T, timeout time.Duration) {
	previous := terminalCloseTimeout
	terminalCloseTimeout = timeout
	t.Cleanup(func() { terminalCloseTimeout = previous })
}

func TestSessionCloseReturnsWhenTerminalNeverExits(t *testing.T) {
	withCloseTimeout(t, 50*time.Millisecond)
	backend := newStuckBackend(false)
	defer close(backend.release)
	session := startTestSession(backend)
	started := time.Now()
	if err := session.Close(); err == nil {
		t.Fatal("Close reported success for a terminal that never exited")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Close took %v; it must be bounded by the close timeout", elapsed)
	}
}

func TestSessionCloseWaitsForNormalExit(t *testing.T) {
	withCloseTimeout(t, 5*time.Second)
	session := startTestSession(newStuckBackend(true))
	if err := session.Close(); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
	select {
	case <-session.done:
	default:
		t.Fatal("Close returned before the session finished")
	}
}

func TestWaitWithTimeout(t *testing.T) {
	want := errors.New("exit status 1")
	if err := waitWithTimeout(func() error { return want }, time.Second); err != want {
		t.Fatalf("waitWithTimeout = %v, want %v", err, want)
	}
	block := make(chan struct{})
	defer close(block)
	if err := waitWithTimeout(func() error { <-block; return nil }, 10*time.Millisecond); err == nil {
		t.Fatal("waitWithTimeout did not time out")
	}
}
