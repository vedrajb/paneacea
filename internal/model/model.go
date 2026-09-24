package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"time"
)

type Layout struct {
	PaneID      string  `json:"paneId,omitempty"`
	Orientation string  `json:"orientation,omitempty"`
	Ratio       float64 `json:"ratio,omitempty"`
	First       *Layout `json:"first,omitempty"`
	Second      *Layout `json:"second,omitempty"`
}
type Workspace struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	RootDirectory string    `json:"rootDirectory"`
	Tabs          []*Tab    `json:"tabs"`
	ActiveTabID   string    `json:"activeTabId"`
	CreatedAt     time.Time `json:"createdAt"`
	LastOpenedAt  time.Time `json:"lastOpenedAt"`
}
type Tab struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspaceId"`
	Title          string    `json:"title"`
	TitleMode      string    `json:"titleMode"`
	RootLayoutNode *Layout   `json:"rootLayoutNode"`
	ActivePaneID   string    `json:"activePaneId"`
	CreatedAt      time.Time `json:"createdAt"`
	SortOrder      int       `json:"sortOrder"`
}
type Agent struct {
	Type              string   `json:"type"`
	State             string   `json:"state"`
	SessionID         string   `json:"sessionId"`
	RootPID           uint32   `json:"rootPid"`
	ProcessGeneration string   `json:"processGeneration"`
	Executable        string   `json:"executable"`
	Arguments         []string `json:"arguments"`
}
type Pane struct {
	ID                      string            `json:"id"`
	WorkspaceID             string            `json:"workspaceId"`
	TabID                   string            `json:"tabId"`
	ProfileID               string            `json:"profileId"`
	Executable              string            `json:"executable"`
	RunningProgram          string            `json:"runningProgram,omitempty"`
	Arguments               []string          `json:"arguments"`
	Environment             map[string]string `json:"environment"`
	InitialWorkingDirectory string            `json:"initialWorkingDirectory"`
	CurrentWorkingDirectory string            `json:"currentWorkingDirectory"`
	RuntimeTerminalID       string            `json:"runtimeTerminalId"`
	PID                     uint32            `json:"pid"`
	Agent                   *Agent            `json:"agent,omitempty"`
	Title                   string            `json:"title"`
	Status                  string            `json:"status"`
	Error                   string            `json:"error,omitempty"`
	Columns                 uint16            `json:"columns"`
	Rows                    uint16            `json:"rows"`
}
type Profile struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
	Available  bool     `json:"available"`
}
type Settings struct {
	DefaultShell         Profile           `json:"defaultShell"`
	Scrollback           int               `json:"scrollback"`
	TerminalHistoryLines int               `json:"terminalHistoryLines"`
	FontSize             int               `json:"fontSize"`
	Theme                string            `json:"theme"`
	Keybindings          map[string]string `json:"keybindings"`
}
type State struct {
	Version           int              `json:"version"`
	Revision          uint64           `json:"revision"`
	Workspaces        []*Workspace     `json:"workspaces"`
	Panes             map[string]*Pane `json:"panes"`
	ActiveWorkspaceID string           `json:"activeWorkspaceId"`
	Settings          Settings         `json:"settings"`
}

func NewState() *State {
	return &State{Version: 1, Workspaces: []*Workspace{}, Panes: map[string]*Pane{}, Settings: Settings{Scrollback: 10000, TerminalHistoryLines: 2000, FontSize: 13, Theme: "dark", Keybindings: map[string]string{}}}
}
func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (n *Layout) Find(id string) *Layout {
	if n == nil {
		return nil
	}
	if n.PaneID == id {
		return n
	}
	if f := n.First.Find(id); f != nil {
		return f
	}
	return n.Second.Find(id)
}
func (n *Layout) Leaves() []string {
	if n == nil {
		return nil
	}
	if n.PaneID != "" {
		return []string{n.PaneID}
	}
	return append(n.First.Leaves(), n.Second.Leaves()...)
}
func (n *Layout) Remove(id string) *Layout {
	if n == nil || n.PaneID == id {
		return nil
	}
	if n.PaneID != "" {
		return n
	}
	n.First = n.First.Remove(id)
	n.Second = n.Second.Remove(id)
	if n.First == nil {
		return n.Second
	}
	if n.Second == nil {
		return n.First
	}
	return n
}
func (n *Layout) At(path []int) (*Layout, error) {
	for _, p := range path {
		if n == nil || n.PaneID != "" {
			return nil, fmt.Errorf("invalid split path")
		}
		if p == 0 {
			n = n.First
		} else if p == 1 {
			n = n.Second
		} else {
			return nil, fmt.Errorf("path must contain 0 or 1")
		}
	}
	if n == nil || n.PaneID != "" {
		return nil, fmt.Errorf("split not found")
	}
	return n, nil
}
func ValidRatio(r float64) bool { return !math.IsNaN(r) && r >= 0.1 && r <= 0.9 }
func (s *State) Workspace(id string) *Workspace {
	for _, w := range s.Workspaces {
		if w.ID == id {
			return w
		}
	}
	return nil
}
func (s *State) Tab(id string) (*Workspace, *Tab) {
	for _, w := range s.Workspaces {
		for _, t := range w.Tabs {
			if t.ID == id {
				return w, t
			}
		}
	}
	return nil, nil
}
