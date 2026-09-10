package daemon

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestOutputPreservesArbitraryBytesAndSequence(t *testing.T) {
	o := newOutput()
	o.append([]byte{0xf0, 0x9f})
	first, err := o.read(context.Background(), 0)
	if err != nil || !bytes.Equal(first.Data, []byte{0xf0, 0x9f}) {
		t.Fatal("bytes changed")
	}
	o.append([]byte{0x98, 0x80})
	second, _ := o.read(context.Background(), first.Sequence)
	if second.Sequence != 4 || !bytes.Equal(second.Data, []byte{0x98, 0x80}) {
		t.Fatal("incremental read lost or duplicated bytes")
	}
}
func TestOutputBoundsAndReportsTruncation(t *testing.T) {
	o := newOutput()
	o.append(bytes.Repeat([]byte("x"), historyLimit+100))
	result, err := o.read(context.Background(), 0)
	if err != nil || !result.Truncated || len(o.data) > historyLimit || len(result.Data) > 65536 {
		t.Fatal("output was not bounded")
	}
}
func TestOutputCancellationAndExit(t *testing.T) {
	o := newOutput()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := o.read(ctx, 0); err == nil {
		t.Fatal("read did not cancel")
	}
	o.exit()
	result, err := o.read(context.Background(), 0)
	if err != nil || !result.Exited {
		t.Fatal("exit did not wake reader")
	}
}
