package daemon

import (
	"strings"
	"testing"
)

func TestConsumeMetadataAcrossChunks(t *testing.T) {
	input := "\x1b]7;file://localhost/C:/workspace/other\a"
	want := "7;file://localhost/C:/workspace/other"
	for split := 0; split <= len(input); split++ {
		values, remainder := consumeMetadata(input[:split])
		more, remainder := consumeMetadata(remainder + input[split:])
		values = append(values, more...)
		if len(values) != 1 || values[0] != want || remainder != "" {
			t.Fatalf("split %d produced values=%q remainder=%q", split, values, remainder)
		}
	}
}

func TestConsumeMetadataSupportsStringTerminator(t *testing.T) {
	prefix := "\x1b]7;file://localhost/C:/workspace/other"
	values, remainder := consumeMetadata(prefix + "\x1b")
	if len(values) != 0 || remainder != prefix+"\x1b" {
		t.Fatalf("incomplete string terminator produced values=%q remainder=%q", values, remainder)
	}
	values, remainder = consumeMetadata(remainder + "\\")
	if len(values) != 1 || values[0] != "7;file://localhost/C:/workspace/other" || remainder != "" {
		t.Fatalf("string terminator produced values=%q remainder=%q", values, remainder)
	}
}

func TestConsumeMetadataDropsOversizedSequences(t *testing.T) {
	oversized := "\x1b]7;" + strings.Repeat("x", 8192)
	values, remainder := consumeMetadata(oversized + "\a")
	if len(values) != 0 || remainder != "" {
		t.Fatalf("terminated oversized sequence produced values=%q remainder=%q", values, remainder)
	}
	values, remainder = consumeMetadata(oversized)
	if len(values) != 0 || remainder != "" {
		t.Fatalf("unterminated oversized sequence produced values=%q remainder=%q", values, remainder)
	}
}
