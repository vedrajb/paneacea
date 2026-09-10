package terminal

import (
	"sync"
	"time"
)

const (
	DefaultBatchInterval = 6 * time.Millisecond
	DefaultBatchBytes    = 64 * 1024
)

type OutputBatcher struct {
	mu        sync.Mutex
	emitMu    sync.Mutex
	buffer    []byte
	closed    bool
	maxBytes  int
	emit      func([]byte)
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewOutputBatcher(interval time.Duration, maxBytes int, emit func([]byte)) *OutputBatcher {
	if interval <= 0 {
		interval = DefaultBatchInterval
	}
	if maxBytes <= 0 {
		maxBytes = DefaultBatchBytes
	}
	batcher := &OutputBatcher{
		buffer:   make([]byte, 0, maxBytes),
		maxBytes: maxBytes,
		emit:     emit,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go batcher.run(interval)
	return batcher
}

func (b *OutputBatcher) Add(data []byte) {
	if len(data) == 0 {
		return
	}
	b.emitMu.Lock()
	defer b.emitMu.Unlock()
	var payloads [][]byte
	b.mu.Lock()
	if !b.closed {
		b.buffer = append(b.buffer, data...)
		for len(b.buffer) >= b.maxBytes {
			payloads = append(payloads, append([]byte(nil), b.buffer[:b.maxBytes]...))
			b.buffer = b.buffer[b.maxBytes:]
		}
	}
	b.mu.Unlock()
	b.emitAll(payloads)
}

func (b *OutputBatcher) Flush() {
	b.emitMu.Lock()
	defer b.emitMu.Unlock()
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return
	}
	payload := append([]byte(nil), b.buffer...)
	b.buffer = b.buffer[:0]
	b.mu.Unlock()
	b.emitAll([][]byte{payload})
}

func (b *OutputBatcher) Close() {
	b.closeOnce.Do(func() {
		b.emitMu.Lock()
		b.mu.Lock()
		b.closed = true
		var payload []byte
		if len(b.buffer) > 0 {
			payload = append([]byte(nil), b.buffer...)
			b.buffer = b.buffer[:0]
		}
		b.mu.Unlock()
		b.emitAll([][]byte{payload})
		close(b.stop)
		b.emitMu.Unlock()
		<-b.done
	})
}

func (b *OutputBatcher) run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer close(b.done)
	for {
		select {
		case <-ticker.C:
			b.Flush()
		case <-b.stop:
			b.Flush()
			return
		}
	}
}

func (b *OutputBatcher) emitAll(payloads [][]byte) {
	if b.emit == nil {
		return
	}
	for _, payload := range payloads {
		if len(payload) > 0 {
			b.emit(payload)
		}
	}
}
