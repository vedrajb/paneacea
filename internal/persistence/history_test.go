package persistence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/paneacea/paneacea/internal/model"
)

type testHistoryProtector struct{}

func (testHistoryProtector) Protect(data []byte) ([]byte, error) {
	result := append([]byte("protected:"), data...)
	for i := len("protected:"); i < len(result); i++ {
		result[i] ^= 0x5a
	}
	return result, nil
}

func (testHistoryProtector) Unprotect(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte("protected:")) {
		return nil, fmt.Errorf("invalid protected value")
	}
	result := append([]byte(nil), data[len("protected:"):]...)
	for i := range result {
		result[i] ^= 0x5a
	}
	return result, nil
}

func TestLegacyStateGetsDefaultSavedHistorySetting(t *testing.T) {
	state := model.NewState()
	data := []byte(`{"version":1,"workspaces":[],"panes":{},"settings":{"scrollback":10000,"fontSize":13,"theme":"dark","keybindings":{}}}`)
	if err := json.Unmarshal(data, state); err != nil {
		t.Fatal(err)
	}
	if state.Settings.TerminalHistoryLines != 2000 {
		t.Fatalf("legacy history setting = %d, want 2000", state.Settings.TerminalHistoryLines)
	}
}

func TestTerminalHistoryIsProtectedAndSurvivesStateSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := OpenWithHistoryProtector(path, testHistoryProtector{})
	if err != nil {
		t.Fatal(err)
	}
	history := TerminalHistory{Version: 1, Columns: 100, Rows: 30, Data: []byte("\x1b[31mHISTORY\x1b[0m")}
	if err = store.SaveTerminalHistory("pane-a", history); err != nil {
		t.Fatal(err)
	}
	state := model.NewState()
	state.Settings.TerminalHistoryLines = 500
	if err = store.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadTerminalHistory("pane-a")
	if err != nil || loaded.Columns != history.Columns || loaded.Rows != history.Rows || !bytes.Equal(loaded.Data, history.Data) {
		t.Fatalf("terminal history was not preserved: history=%#v err=%v", loaded, err)
	}
	var stored []byte
	if err = store.db.QueryRow(`SELECT data FROM terminal_history WHERE pane_id=?`, "pane-a").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, history.Data) {
		t.Fatal("terminal history was stored without protection")
	}
	if err = store.DeleteTerminalHistoryExcept([]string{"pane-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.LoadTerminalHistory("pane-a"); err == nil {
		t.Fatal("orphaned terminal history was not removed")
	}
	store.Close()
}

func TestTerminalHistoryRejectsOversizedAndInvalidRecords(t *testing.T) {
	store, err := OpenWithHistoryProtector(filepath.Join(t.TempDir(), "state.db"), testHistoryProtector{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.SaveTerminalHistory("pane-a", TerminalHistory{Version: 1, Columns: 100, Rows: 30, Data: make([]byte, MaxTerminalHistoryBytes+1)}); err == nil {
		t.Fatal("oversized terminal history was accepted")
	}
	if err = store.SaveTerminalHistory("pane-a", TerminalHistory{Version: 2, Columns: 100, Rows: 30, Data: []byte("bad")}); err == nil {
		t.Fatal("unsupported terminal history version was accepted")
	}
}
