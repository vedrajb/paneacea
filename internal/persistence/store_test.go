package persistence

import (
	"github.com/paneacea/paneacea/internal/model"
	"path/filepath"
	"testing"
)

func TestStateSurvivesStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	state := model.NewState()
	state.Settings.FontSize = 17
	state.Workspaces = append(state.Workspaces, &model.Workspace{ID: "w", Name: "Project", RootDirectory: "C:\\dev"})
	if err = store.Save(state); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.FontSize != 17 || loaded.Workspaces[0].Name != "Project" {
		t.Fatal("state did not survive reopen")
	}
}
