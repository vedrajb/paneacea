package persistence

import (
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/paneacea/paneacea/internal/model"
	_ "modernc.org/sqlite"
	"path/filepath"
	"strings"
)

const MaxTerminalHistoryBytes = 2 * 1024 * 1024

type TerminalHistory struct {
	Version int    `json:"version"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
	Data    []byte `json:"data"`
}

type HistoryProtector interface {
	Protect([]byte) ([]byte, error)
	Unprotect([]byte) ([]byte, error)
}

type Store struct {
	db        *sql.DB
	protector HistoryProtector
	path      string
}

func Open(path string) (*Store, error) {
	return OpenWithHistoryProtector(path, systemHistoryProtector{})
}

func OpenWithHistoryProtector(path string, protector HistoryProtector) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS application_state (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL, data BLOB NOT NULL); CREATE TABLE IF NOT EXISTS terminal_history (pane_id TEXT PRIMARY KEY, format_version INTEGER NOT NULL, data BLOB NOT NULL, updated_at INTEGER NOT NULL);`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, protector: protector, path: path}, nil
}

func (s *Store) BashHistoryPath(paneID string) (string, error) {
	decoded, err := hex.DecodeString(paneID)
	if err != nil || len(decoded) != 16 {
		return "", fmt.Errorf("invalid pane ID")
	}
	return filepath.Join(filepath.Dir(s.path), "bash-history", paneID), nil
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

func (s *Store) SaveTerminalHistory(paneID string, history TerminalHistory) error {
	if paneID == "" || history.Version != 1 || history.Columns < 1 || history.Columns > 1000 || history.Rows < 1 || history.Rows > 500 || len(history.Data) > MaxTerminalHistoryBytes {
		return fmt.Errorf("invalid terminal history")
	}
	plain := make([]byte, 16+len(history.Data))
	copy(plain[:4], "PTH1")
	binary.BigEndian.PutUint16(plain[4:6], uint16(history.Version))
	binary.BigEndian.PutUint16(plain[6:8], uint16(history.Columns))
	binary.BigEndian.PutUint16(plain[8:10], uint16(history.Rows))
	binary.BigEndian.PutUint32(plain[10:14], uint32(len(history.Data)))
	copy(plain[16:], history.Data)
	encrypted, err := s.protector.Protect(plain)
	if err != nil {
		return fmt.Errorf("protect terminal history: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO terminal_history(pane_id,format_version,data,updated_at) VALUES(?,1,?,unixepoch()) ON CONFLICT(pane_id) DO UPDATE SET format_version=excluded.format_version,data=excluded.data,updated_at=excluded.updated_at`, paneID, encrypted)
	return err
}

func (s *Store) LoadTerminalHistory(paneID string) (*TerminalHistory, error) {
	var version int
	var encrypted []byte
	err := s.db.QueryRow(`SELECT format_version,data FROM terminal_history WHERE pane_id=?`, paneID).Scan(&version, &encrypted)
	if err != nil {
		return nil, err
	}
	if version != 1 || len(encrypted) == 0 || len(encrypted) > MaxTerminalHistoryBytes+64*1024 {
		return nil, fmt.Errorf("invalid stored terminal history")
	}
	plain, err := s.protector.Unprotect(encrypted)
	if err != nil {
		return nil, fmt.Errorf("unprotect terminal history: %w", err)
	}
	if len(plain) < 16 || string(plain[:4]) != "PTH1" || int(binary.BigEndian.Uint32(plain[10:14])) != len(plain)-16 {
		return nil, fmt.Errorf("decode terminal history: invalid header")
	}
	history := TerminalHistory{Version: int(binary.BigEndian.Uint16(plain[4:6])), Columns: int(binary.BigEndian.Uint16(plain[6:8])), Rows: int(binary.BigEndian.Uint16(plain[8:10])), Data: append([]byte(nil), plain[16:]...)}
	if history.Version != 1 || history.Columns < 1 || history.Columns > 1000 || history.Rows < 1 || history.Rows > 500 || len(history.Data) == 0 || len(history.Data) > MaxTerminalHistoryBytes {
		return nil, fmt.Errorf("invalid terminal history payload")
	}
	return &history, nil
}

func (s *Store) DeleteTerminalHistory(paneID string) error {
	_, err := s.db.Exec(`DELETE FROM terminal_history WHERE pane_id=?`, paneID)
	return err
}

func (s *Store) DeleteAllTerminalHistory() error {
	_, err := s.db.Exec(`DELETE FROM terminal_history`)
	return err
}

func (s *Store) DeleteTerminalHistoryExcept(paneIDs []string) error {
	if len(paneIDs) == 0 {
		return s.DeleteAllTerminalHistory()
	}
	query := `DELETE FROM terminal_history WHERE pane_id NOT IN (` + strings.TrimRight(strings.Repeat("?,", len(paneIDs)), ",") + `)`
	args := make([]any, len(paneIDs))
	for i, id := range paneIDs {
		args[i] = id
	}
	_, err := s.db.Exec(query, args...)
	return err
}

func (s *Store) Close() error { return s.db.Close() }
