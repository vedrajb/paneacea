package terminal

import (
	"testing"
	"time"
)

func TestOutputBatcherKeepsCursorRepairWithRedraw(t *testing.T) {
	received := make(chan string, 16)
	batcher := NewOutputBatcher(5*time.Millisecond, 65536, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	batcher.Add([]byte("\x1b[?2026h\x1b[30;1H\x1b[?25h\x1b[?2026l"))
	time.Sleep(30 * time.Millisecond)
	select {
	case value := <-received:
		t.Fatalf("redraw flushed before its cursor repair: %q", value)
	default:
	}
	batcher.Add([]byte("\x1b[?25l\x1b[34;3H\x1b[?25h"))
	select {
	case value := <-received:
		if value != "\x1b[?2026h\x1b[30;1H\x1b[?25h\x1b[?2026l\x1b[?25l\x1b[34;3H\x1b[?25h" {
			t.Fatalf("redraw and cursor repair were separated: %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for repaired redraw")
	}
}

func TestOutputBatcherFlushesAfterSynchronizedOutputDeadline(t *testing.T) {
	received := make(chan string, 16)
	batcher := NewOutputBatcher(5*time.Millisecond, 65536, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	batcher.Add([]byte("\x1b[?2026hframe\x1b[?2026l"))
	select {
	case value := <-received:
		if value != "\x1b[?2026hframe\x1b[?2026l" {
			t.Fatalf("flushed value = %q", value)
		}
	case <-time.After(SynchronizedOutputRepairDelay + time.Second):
		t.Fatal("synchronized output did not flush after its repair deadline")
	}
}

func TestOutputBatcherExplicitFlushDoesNotWaitForQuiet(t *testing.T) {
	received := make(chan string, 16)
	batcher := NewOutputBatcher(time.Hour, 65536, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	batcher.Add([]byte("explicit flush"))
	batcher.Flush()
	select {
	case value := <-received:
		if value != "explicit flush" {
			t.Fatalf("flushed value = %q", value)
		}
	default:
		t.Fatal("explicit flush waited for quiet output")
	}
}
