package persistence

import (
	"database/sql"
	"encoding/json"
	"github.com/paneacea/paneacea/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS application_state (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL, data BLOB NOT NULL);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Load() (*model.State, error) {
	var data []byte
	err := s.db.QueryRow("SELECT data FROM application_state WHERE id=1").Scan(&data)
	if err == sql.ErrNoRows {
		return model.NewState(), nil
	}
	if err != nil {
		return nil, err
	}
	state := model.NewState()
	if err = json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	return state, nil
}
func (s *Store) Save(state *model.State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO application_state(id,version,data) VALUES(1,1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", data)
	return err
}
func (s *Store) Close() error { return s.db.Close() }
