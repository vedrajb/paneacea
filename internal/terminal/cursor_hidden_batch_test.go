package terminal

import (
	"testing"
	"time"
)

func TestOutputBatcherDoesNotHoldSynchronizedOutputWhileCursorHidden(t *testing.T) {
	received := make(chan string, 16)
	batcher := NewOutputBatcher(5*time.Millisecond, 65536, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	batcher.Add([]byte("\x1b[?25l"))
	<-received
	started := time.Now()
	batcher.Add([]byte("\x1b[?2026h\x1b[?2026l\x1b[27;1Hframe"))
	select {
	case value := <-received:
		if value != "\x1b[?2026h\x1b[?2026l\x1b[27;1Hframe" {
			t.Fatalf("flushed value = %q", value)
		}
		if elapsed := time.Since(started); elapsed >= SynchronizedOutputRepairDelay {
			t.Fatalf("hidden-cursor frame waited %v for a cursor repair", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for hidden-cursor frame")
	}
}

func TestOutputBatcherHiddenCursorFramesStreamWhileKeyRepeats(t *testing.T) {
	received := make(chan string, 64)
	batcher := NewOutputBatcher(5*time.Millisecond, 65536, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	batcher.Add([]byte("\x1b[?25l"))
	<-received
	for i := 0; i < 10; i++ {
		batcher.Add([]byte("\x1b[?2026h\x1b[?2026l\rframe"))
		time.Sleep(20 * time.Millisecond)
	}
	if count := len(received); count < 5 {
		t.Fatalf("only %d flushes for 10 repeated frames; output was held until input stopped", count)
	}
}

func TestOutputBatcherDoesNotExtendCursorRepairDeadlineForEachFrame(t *testing.T) {
	received := make(chan string, 64)
	batcher := NewOutputBatcher(5*time.Millisecond, 65536, func(data []byte) {
		received <- string(data)
	})
	defer batcher.Close()
	started := time.Now()
	deadline := time.After(4 * SynchronizedOutputRepairDelay)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-received:
			if elapsed := time.Since(started); elapsed > 3*SynchronizedOutputRepairDelay {
				t.Fatalf("first flush took %v while frames kept arriving", elapsed)
			}
			return
		case <-ticker.C:
			batcher.Add([]byte("\x1b[?2026h\x1b[1;1H\x1b[?25hframe\x1b[?2026l"))
		case <-deadline:
			t.Fatal("visible-cursor frames were held for as long as frames kept arriving")
		}
	}
}

func TestCursorHiddenAfterUsesLastVisibilityChange(t *testing.T) {
	cases := []struct {
		sequence string
		hidden   bool
		want     bool
	}{
		{"plain", true, true},
		{"plain", false, false},
		{"\x1b[?25l", false, true},
		{"\x1b[?25h", true, false},
		{"\x1b[?25h text \x1b[?25l", false, true},
		{"\x1b[?25l text \x1b[?25h", true, false},
	}
	for _, c := range cases {
		if got := cursorHiddenAfter([]byte(c.sequence), c.hidden); got != c.want {
			t.Fatalf("cursorHiddenAfter(%q, %v) = %v, want %v", c.sequence, c.hidden, got, c.want)
		}
	}
}
