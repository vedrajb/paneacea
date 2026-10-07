package daemon

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

func newModifyOtherKeysOutput() *outputBuffer {
	output := newOutput()
	output.emulator = vt.NewEmulator(40, 10)
	output.modes = map[ansi.Mode]bool{}
	output.emulator.RegisterCsiHandler(ansi.Command('>', 0, 'm'), output.trackModifyOtherKeys)
	return output
}

func TestSnapshotRestoresModifyOtherKeys(t *testing.T) {
	output := newModifyOtherKeysOutput()
	output.append([]byte("prompt\x1b[>4;2m"))
	if data := string(output.snapshot().Data); !strings.HasSuffix(data, "\x1b[>4;2m") {
		t.Fatalf("snapshot does not restore modifyOtherKeys: %q", data)
	}
}

func TestSnapshotOmitsDisabledModifyOtherKeys(t *testing.T) {
	for _, disable := range []string{"\x1b[>4;0m", "\x1b[>4m"} {
		output := newModifyOtherKeysOutput()
		output.append([]byte("\x1b[>4;2m" + disable))
		if data := string(output.snapshot().Data); strings.Contains(data, "\x1b[>4;") {
			t.Fatalf("snapshot after %q restores modifyOtherKeys: %q", disable, data)
		}
	}
}

func TestModifyOtherKeysIgnoresOtherResources(t *testing.T) {
	output := newModifyOtherKeysOutput()
	output.append([]byte("\x1b[>1;2m"))
	if output.modifyOtherKeys != 0 {
		t.Fatalf("modifyOtherKeys = %d after modifyCursorKeys", output.modifyOtherKeys)
	}
}

func TestPersistedSnapshotOmitsModifyOtherKeys(t *testing.T) {
	output := newModifyOtherKeysOutput()
	output.append([]byte("prompt\x1b[>4;2m"))
	data, _, _, _, err := output.persistedSnapshot(100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\x1b[>4;") {
		t.Fatalf("persisted history restores modifyOtherKeys for a new shell: %q", data)
	}
}
