package daemon

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

func TestRestoredScreenIsArchivedBeforeStartupClear(t *testing.T) {
	output := newOutput()
	output.emulator = vt.NewEmulator(40, 10)
	output.emulator.SetScrollbackSize(100)
	if err := output.restore([]byte("\x1b[31mOLD_FIRST\x1b[0m\r\n\r\nOLD_LAST")); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(ansi.Strip(output.emulator.Render())) != "" {
		t.Fatal("saved screen remains exposed to the new shell's repaint")
	}
	if got := output.scrollbackLines(); got != 3 {
		t.Fatalf("archived %d lines, want 3 including the interior blank line", got)
	}
	output.append([]byte("\x1b[H\x1b[JNEW_PROMPT"))
	attached := output.attachSnapshot(0)
	for _, marker := range []string{"OLD_FIRST", "OLD_LAST", "NEW_PROMPT"} {
		if !bytes.Contains(attached.Data, []byte(marker)) {
			t.Fatalf("startup repaint lost %s", marker)
		}
	}
	if !bytes.Contains(attached.Data, []byte("\x1b[31m")) {
		t.Fatal("archiving lost the saved text color")
	}
}

func TestPersistedSnapshotRestoresHistoryAndOmitsAlternateScreen(t *testing.T) {
	output := newOutput()
	output.emulator = vt.NewEmulator(40, 4)
	output.emulator.SetScrollbackSize(100)
	output.modes = map[ansi.Mode]bool{}
	output.emulator.SetCallbacks(vt.Callbacks{EnableMode: func(mode ansi.Mode) { output.modes[mode] = true }, DisableMode: func(mode ansi.Mode) { output.modes[mode] = false }})
	output.append([]byte("HISTORY_COLOR_Ω\x1b[31m RED\x1b[0m\r\nSECOND_LINE\r\nTHIRD_LINE\r\nFOURTH_LINE\r\n"))
	saved, columns, rows, generation, err := output.persistedSnapshot(100, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	restored := newOutput()
	restored.emulator = vt.NewEmulator(columns, rows)
	restored.emulator.SetScrollbackSize(100)
	restored.modes = map[ansi.Mode]bool{}
	restored.emulator.SetCallbacks(vt.Callbacks{EnableMode: func(mode ansi.Mode) { restored.modes[mode] = true }, DisableMode: func(mode ansi.Mode) { restored.modes[mode] = false }})
	if err = restored.restore(saved); err != nil {
		t.Fatal(err)
	}
	got := restored.snapshot()
	if !got.Restored || !bytes.Contains(got.Data, []byte("HISTORY_COLOR_Ω")) || !bytes.Contains(got.Data, []byte("SECOND_LINE")) {
		t.Fatalf("persisted terminal history was not restored: %q", got.Data)
	}
	output.append([]byte("\x1b[?1049hALT_SCREEN_SHOULD_NOT_RETURN"))
	saved, _, _, _, err = output.persistedSnapshot(100, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte("ALT_SCREEN_SHOULD_NOT_RETURN")) {
		t.Fatal("alternate-screen application content was saved as terminal history")
	}
	if generation == 0 {
		t.Fatal("output changes were not marked dirty for checkpointing")
	}
}
