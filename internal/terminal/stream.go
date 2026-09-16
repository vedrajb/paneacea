package terminal

import (
	"bytes"
	"sync"
	"time"
)

const (
	DefaultBatchInterval          = 6 * time.Millisecond
	DefaultBatchBytes             = 64 * 1024
	SynchronizedOutputRepairDelay = 50 * time.Millisecond
)

var (
	synchronizedOutputEnd = []byte("\x1b[?2026l")
	showCursor            = []byte("\x1b[?25h")
)

type OutputBatcher struct {
	mu             sync.Mutex
	emitMu         sync.Mutex
	buffer         []byte
	sequenceTail   []byte
	syncDeadline   time.Time
	syncEndPending bool
	closed         bool
	maxBytes       int
	emit           func([]byte)
	stop           chan struct{}
	done           chan struct{}
	closeOnce      sync.Once
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
		b.trackSynchronizedOutput(data, time.Now())
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
			b.flushOnTimer()
		case <-b.stop:
			b.Flush()
			return
		}
	}
}

func (b *OutputBatcher) flushOnTimer() {
	b.emitMu.Lock()
	defer b.emitMu.Unlock()
	b.mu.Lock()
	if len(b.buffer) == 0 || (!b.syncDeadline.IsZero() && time.Now().Before(b.syncDeadline)) {
		b.mu.Unlock()
		return
	}
	payload := append([]byte(nil), b.buffer...)
	b.buffer = b.buffer[:0]
	b.syncDeadline = time.Time{}
	b.syncEndPending = false
	b.mu.Unlock()
	b.emitAll([][]byte{payload})
}

func (b *OutputBatcher) trackSynchronizedOutput(data []byte, now time.Time) {
	sequence := append(append([]byte(nil), b.sequenceTail...), data...)
	if b.syncEndPending && bytes.Contains(sequence, showCursor) {
		b.syncDeadline = time.Time{}
		b.syncEndPending = false
	}
	if end := bytes.LastIndex(sequence, synchronizedOutputEnd); end >= 0 {
		b.syncDeadline = now.Add(SynchronizedOutputRepairDelay)
		b.syncEndPending = true
		if bytes.Contains(sequence[end+len(synchronizedOutputEnd):], showCursor) {
			b.syncDeadline = time.Time{}
			b.syncEndPending = false
		}
	}
	if len(sequence) > len(synchronizedOutputEnd)-1 {
		sequence = sequence[len(sequence)-(len(synchronizedOutputEnd)-1):]
	}
	b.sequenceTail = append(b.sequenceTail[:0], sequence...)
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
