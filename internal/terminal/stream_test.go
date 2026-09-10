package terminal

import (
	"testing"
	"time"
)

func TestOutputBatcherFlushesAtByteThreshold(t *testing.T) {
	received := make(chan string, 4)
	batcher := NewOutputBatcher(time.Hour, 3, func(data []byte) {
		received <- string(data)
	})

	batcher.Add([]byte("ab"))
	select {
	case value := <-received:
		t.Fatalf("received a batch before the threshold: %q", value)
	default:
	}
	batcher.Add([]byte("cdef"))
	if first := <-received; first != "abc" {
		t.Fatalf("first batch = %q, want %q", first, "abc")
	}
	if second := <-received; second != "def" {
		t.Fatalf("second batch = %q, want %q", second, "def")
	}
	batcher.Close()
}

func TestOutputBatcherCloseFlushesPendingOutput(t *testing.T) {
	received := make(chan string, 1)
	batcher := NewOutputBatcher(time.Hour, 64, func(data []byte) {
		received <- string(data)
	})
	batcher.Add([]byte("pending"))
	batcher.Close()
	select {
	case value := <-received:
		if value != "pending" {
			t.Fatalf("flushed value = %q, want %q", value, "pending")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for final batch")
	}
	batcher.Add([]byte("ignored"))
	select {
	case value := <-received:
		t.Fatalf("received output after close: %q", value)
	default:
	}
}

func TestOutputBatcherFlushesOnTimer(t *testing.T) {
	received := make(chan string, 1)
	batcher := NewOutputBatcher(10*time.Millisecond, 64, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	batcher.Add([]byte("timer"))
	select {
	case value := <-received:
		if value != "timer" {
			t.Fatalf("timer value = %q, want %q", value, "timer")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for timer batch")
	}
}
