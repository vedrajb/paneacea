# Paneacea — Lightweight Windows Terminal Workspace Application Plan

# Paneacea Project Naming

## Project positioning

Paneacea is a **personal open-source project intended for public use**. The repository can be public and community-facing while remaining an independently maintained personal project.

## Naming convention

Use the following naming split consistently:

```text
GitHub repository / project slug:  tinkershell
Product / display name:             Paneacea
CLI / executable:                   tinkershell
Optional short CLI alias:           tshell
```

Recommended GitHub repository URL shape:

```text
github.com/<user-or-org>/tinkershell
```

Use `Paneacea` in README headings, release notes, screenshots, UI chrome, documentation titles, and branding. Keep the existing lowercase `tinkershell` repository, package, and developer CLI identifiers for compatibility.

---

## 1. Goal

Build a lightweight Windows-first terminal application with:

- Tabs
- Horizontal and vertical pane splits
- Pane resizing
- Tab renaming
- Persistent workspaces
- Workspace-specific root directories
- Workspace navigation
- Saved tab and pane layouts
- Windows Terminal-compatible keybindings
- Persistent terminal sessions
- AI-agent detection
- AI-agent session restoration where supported
- Low idle CPU and memory usage

The application should combine the strongest architectural ideas from:

- Windows Terminal
- tmux
- abduco
- dvtm
- Herdr
- RMUX
- WezTerm

---

# 2. Recommended Stack

## Runtime / Multiplexer Backend

Use **Rust**.

Recommended components:

```text
Rust
├── Tokio
├── portable-pty or direct ConPTY integration
├── windows-rs
├── SQLite
├── Named Pipes
└── Agent adapters
```

Responsibilities:

- Own PTY/ConPTY sessions
- Keep terminal processes alive independently of the GUI
- Manage workspaces, tabs, panes and sessions
- Track process trees
- Detect AI agents
- Persist workspace/session metadata
- Expose an IPC API to the GUI
- Restore agent sessions after restart where supported

---

## GUI

Use:

```text
C#
.NET
WPF
Microsoft.Terminal.Wpf
```

The GUI should be a client of the Rust runtime rather than the process owner.

Responsibilities:

- Main window
- Workspace sidebar
- Tabs
- Pane layout
- Terminal rendering
- Command palette
- Context menus
- Keybindings
- Agent indicators
- User settings

---

## UI Direction — VS Code-Inspired Presentation

The user interface should use a VS Code-inspired language and visual system while remaining a native WPF application. This is a presentation decision; it does not change the Rust runtime, ConPTY ownership, SQLite persistence, named-pipe protocol, or `Microsoft.Terminal.Wpf` renderer.

### UI language

Use familiar terms consistently:

| Paneacea concept | Display language |
|---|---|
| Workspace collection | Explorer |
| Workspace | Workspace |
| Workspace root directory | Workspace Folder |
| Tab collection | Terminal Tabs |
| Pane tree | Terminal Layout |
| Command list | Command Palette |
| Runtime connection state | Status Bar |
| Saved terminal process | Session |

Prefer action names that read like VS Code commands:

```text
Terminal: New Tab
Terminal: Split Right
Terminal: Split Down
Terminal: Focus Previous Pane
Terminal: Resize Pane
Terminal: Close Pane
Workspace: New Workspace
Workspace: Open Folder
Workspace: Rename Workspace
Workspace: Change Workspace Folder
Preferences: Open Settings
Paneacea: Reconnect Runtime
```

The command palette should support fuzzy search, keyboard navigation, visible keybinding hints, and category prefixes. Context menus and tooltips should use the same action names as the palette. The application should describe a directory selected for a workspace as a folder in user-facing copy, while the data model may continue to use `rootDirectory`.

### Default shell settings

The first settings panel exposes the default terminal shell. On Windows, detect these profiles:

```text
pwsh       PowerShell 7
powershell Windows PowerShell 5.1
git-bash   Git Bash for Windows
cmd        Command Prompt
```

Persist the selected profile's stable id, executable path, and argument list under `settings.defaultShell`. New tabs and split panes use that profile when they do not provide an explicit launch command. Existing panes retain their saved command so changing the default does not interrupt running sessions. The panel should show the resolved executable path and whether each supported profile is available.

### Layout

Use a VS Code-like shell around the existing terminal content:

```text
┌──────┬──────────────────────────────────────────────────────────────┐
│      │ title bar / menu                                             │
│      ├──────────────────────────────────────────────────────────────┤
│      │ terminal tabs                                                │
│ Act. ├──────────────────────────────────────────────────────────────┤
│ bar  │                                                              │
│      │                 terminal pane layout                         │
│      │                                                              │
│      ├──────────────────────────────────────────────────────────────┤
│      │ status bar                                                   │
└──────┴──────────────────────────────────────────────────────────────┘
```

The left side should contain a narrow activity bar and an Explorer view. The Explorer view shows workspaces, tabs, panes, process names, and agent status when agent support is added. The center remains a terminal-first work area. The bottom status bar shows the active workspace, folder, shell, pane, connection state, and useful runtime messages. The first MVP may combine the activity bar and Explorer into one sidebar, but the view-model boundaries should leave room for the split later.

### Theme tokens

Ship a dark theme first with centralized WPF resource tokens rather than colors embedded in individual views:

```text
editorBackground       #1E1E1E
sidebarBackground      #252526
activityBarBackground  #333333
panelBackground        #181818
tabBackground          #2D2D2D
tabActiveBackground    #1E1E1E
border                 #3F3F46
foreground             #CCCCCC
mutedForeground        #858585
accent                 #0078D4
focus                  #4FC1FF
error                  #F14C4C
warning                #CCA700
success                #89D185
```

Terminal colors remain controlled by the terminal theme passed to `Microsoft.Terminal.Wpf`; surrounding WPF chrome should consume the same semantic resource names where appropriate. Define light-theme equivalents behind the same resource keys later. User-selected themes should be data-driven and must not alter the runtime protocol.

### WPF implementation boundary

Implement the visual direction with WPF `ResourceDictionary` files, styles, templates, vector icons, and view-model state. Keep terminal rendering inside `Microsoft.Terminal.Wpf` controls. Do not introduce WebView2, Tauri, Electron, or a TypeScript UI solely to obtain this look. The command registry, workspace tree model, and theme resource keys should be reusable by future clients, but the first implementation remains the C# WPF client.

### Interaction and accessibility

Use the VS Code-like interaction pattern of keyboard-first commands plus discoverable controls:

- `Ctrl+Shift+P` opens the Command Palette.
- `Ctrl+P` may be reserved for quick workspace, tab, and pane navigation after that picker exists.
- Activity bar buttons expose accessible names and tooltips.
- Focused tabs, panes, splitters, and tree items have a visible focus state.
- Context menus provide the same operations as the command palette.
- Theme contrast must remain readable for text, focus indicators, disabled controls, and status messages.

The visual system should be inspired by VS Code rather than reproduce its branding, icons, or proprietary assets. Product naming remains **Paneacea**.

---

# 3. High-Level Architecture

```text
                TerminalApp.exe
                  C# / WPF
                       │
        Microsoft.Terminal.Wpf renderer
                       │
                  Named Pipe IPC
                       │
                       ▼
               mux-runtime.exe
                    Rust
                       │
       ┌───────────────┼────────────────┐
       │               │                │
   Workspaces          PTYs           Agents
       │               │                │
     SQLite         ConPTY/PTY      Codex/Claude
                       │
                ┌──────┼──────┐
              pwsh    bash    codex
```

The runtime owns terminals.

The GUI only attaches to them.

This lets users close or restart the GUI while terminal sessions continue running.

---

# 4. Why This Architecture

The strongest lightweight multiplexers generally use native code for the core runtime.

Examples:

| Project | Core | UI | Architecture |
|---|---|---|---|
| tmux | C | ncurses | client/server |
| mtm | C | ncursesw | compact local mux |
| dvtm | C | ncurses | presentation-focused |
| abduco | C | terminal | persistent session server |
| Zellij | Rust | TUI | client/server |
| Herdr | Rust | Ratatui/Crossterm | persistent server |
| RMUX | Rust | optional TUI | daemon + IPC |
| WezTerm | Rust | custom GPU GUI | GUI + mux |

The useful pattern is:

```text
native persistent runtime
        +
separate presentation client
```

That is the same philosophy used by tmux and abduco/dvtm.

---

# 5. Why Not TypeScript as the Primary Stack

TypeScript itself is not expensive because it compiles to JavaScript.

The main resource cost comes from the browser runtime required by a TypeScript desktop GUI.

For example:

```text
Tauri
  └── WebView2
       ├── Browser process
       ├── Renderer process
       ├── GPU process
       └── Utility process
```

Tauri is significantly lighter than Electron, but WebView2 still creates a higher baseline memory footprint than a native WPF UI.

For this application, priorities are:

```text
Windows Terminal fidelity
+
low resource usage
+
persistent sessions
+
agent integration
```

Therefore the preferred primary GUI is:

```text
WPF + Microsoft.Terminal.Wpf
```

rather than:

```text
Tauri + TypeScript + xterm.js
```

TypeScript can still be added later for optional clients or tooling.

---

# 6. Architectural References

## Windows Terminal

Borrow:

- Terminal rendering
- Pane split-tree behavior
- Keybinding/action vocabulary
- Profiles
- Settings concepts
- Tab/pane UX
- Command palette behavior

Do not use Windows Terminal as the persistence owner.

---

## tmux

Borrow:

```text
Client
   │
   │ IPC
   ▼
Server
   │
   ├── PTY
   ├── PTY
   └── PTY
```

The backend survives client disconnects.

---

## abduco

Borrow the strict separation between:

- terminal session ownership
- presentation

The session process should continue after the GUI exits.

---

## dvtm

Borrow the idea that pane layout and presentation can remain independent from session persistence.

---

## Herdr

Borrow:

- Workspace → Tab → Pane hierarchy
- Background server architecture
- Agent recognition
- Agent state tracking
- Attach/detach model
- Native agent session restoration
- IPC-oriented architecture

---

## RMUX

Borrow:

- Rust daemon design
- Windows-native persistent multiplexer model
- IPC API
- Client SDK separation

---

## WezTerm

Study:

- terminal rendering architecture
- Rust terminal engine
- pane handling
- mux/server model

Avoid building a completely custom GPU renderer initially because Windows Terminal already solves the hardest rendering problems.

---

# 7. Core Data Model

## Workspace

```text
Workspace
{
    id
    name
    rootDirectory

    tabs[]
    activeTabId

    createdAt
    lastOpenedAt
}
```

The workspace root is the default directory for newly created tabs.

Example:

```text
Workspace:
    Synapse

Root:
    C:\dev\synapse
```

Pressing the new-tab shortcut should open:

```text
pwsh.exe
cwd = C:\dev\synapse
```

---

## Tab

```text
Tab
{
    id
    workspaceId

    title
    titleMode

    rootLayoutNode
    activePaneId

    createdAt
    sortOrder
}
```

Recommended title behavior:

```text
displayTitle =
    manualTitle ?? terminalTitle
```

---

## Pane

```text
Pane
{
    id
    workspaceId
    tabId

    profileId

    executable
    arguments[]
    environment{}

    initialWorkingDirectory
    currentWorkingDirectory

    runtimeTerminalId
    pid

    agent
}
```

Persist the complete launch command rather than only the executable.

Example:

```text
codex --full-auto
```

must restore its original flags when rebuilding the command.

---

# 8. Pane Layout Model

Use the same general approach as Windows Terminal: a recursive binary split tree.

```text
LayoutNode
├── Leaf
│    └── paneId
│
└── Split
     ├── orientation
     ├── ratio
     ├── first
     └── second
```

Example:

```text
              Vertical 60/40
               /          \
          Pane A        Horizontal
                         /       \
                     Pane B     Pane C
```

This supports:

- arbitrary nested splits
- drag resizing
- keyboard resizing
- directional navigation
- exact persistence

---

# 9. Persistence

Use **SQLite**.

Suggested tables:

```text
workspaces
tabs
panes
terminal_sessions
agent_sessions
settings
```

Store recursive pane layouts as JSON inside the tab record.

Example:

```json
{
  "type": "split",
  "orientation": "vertical",
  "ratio": 0.6,
  "first": {
    "type": "pane",
    "paneId": "p1"
  },
  "second": {
    "type": "split",
    "orientation": "horizontal",
    "ratio": 0.5,
    "first": {
      "type": "pane",
      "paneId": "p2"
    },
    "second": {
      "type": "pane",
      "paneId": "p3"
    }
  }
}
```

---

# 10. Two Kinds of Persistence

These must be treated separately.

## Live persistence

```text
GUI closes
   ↓
Runtime stays alive
   ↓
ConPTY stays alive
   ↓
Shell/agent stays alive
```

When the GUI reopens, it attaches to the existing session.

This is the preferred behavior.

---

## Restart persistence

After:

- reboot
- runtime crash
- explicit runtime restart

the application rebuilds:

- workspace layout
- tabs
- panes
- launch commands

For agents that support native session IDs, it should resume the original agent session.

---

# 11. IPC Protocol

Use Windows Named Pipes between GUI and runtime.

Suggested operations:

```text
workspace.list
workspace.create
workspace.close
workspace.rename
workspace.setRoot

tab.create
tab.close
tab.rename
tab.focus

pane.create
pane.split
pane.close
pane.resize
pane.focus
pane.sendInput

terminal.attach
terminal.detach
terminal.resize

agent.get
agent.status
agent.resume
```

Suggested events:

```text
workspace.created
workspace.updated

tab.created
tab.updated
tab.closed

pane.created
pane.updated
pane.closed
pane.output
pane.cwdChanged

agent.detected
agent.statusChanged
agent.sessionChanged
```

---

# 12. Windows Terminal Keybindings

Use an action-based command system.

Examples:

```text
Terminal.NewTab
Terminal.ClosePane

Terminal.SplitPaneAuto
Terminal.SplitPaneDown
Terminal.SplitPaneRight

Terminal.MoveFocusUp
Terminal.MoveFocusDown
Terminal.MoveFocusLeft
Terminal.MoveFocusRight

Terminal.ResizePaneUp
Terminal.ResizePaneDown
Terminal.ResizePaneLeft
Terminal.ResizePaneRight

Terminal.OpenTabRenamer
```

Ship Windows Terminal-compatible defaults.

Examples:

```text
Alt+Shift+D
    split pane

Alt+Shift+-
    split down

Alt+Shift++
    split right

Alt+Arrow
    move focus

Alt+Shift+Arrow
    resize pane

Ctrl+Shift+W
    close pane
```

Later, optionally support importing relevant actions from Windows Terminal's `settings.json`.

---

# 13. Workspace Commands

Add workspace-specific actions above the Windows Terminal action layer.

```text
Workspace.New
Workspace.Close
Workspace.Next
Workspace.Previous
Workspace.Switch
Workspace.Rename
Workspace.ChangeRoot
Workspace.OpenRoot
```

These should be accessible through the command palette.

---

# 14. Agent Detection

Detection should be layered.

## Layer 1: Process Detection

Inspect the foreground process tree.

Examples:

```text
pwsh.exe
 └── codex.exe
```

or:

```text
pwsh.exe
 └── node.exe
      └── claude.exe
```

Store:

```text
pane.agent.type = Codex
pane.agent.state = Working
```

Useful Windows APIs:

- ToolHelp
- Job Objects
- Windows process APIs
- windows-rs

---

# 15. Agent Adapter Interface

Agent-specific logic should not live inside `Pane`.

Use an adapter model.

```text
AgentAdapter
{
    Detect(process)
    GetState(...)
    GetSessionId(...)
    BuildResumeCommand(...)
    SupportsResume
}
```

Initial adapters:

```text
CodexAgentAdapter
ClaudeAgentAdapter
OpenCodeAgentAdapter
CursorAgentAdapter
CopilotAgentAdapter
HermesAgentAdapter
```

---

# 16. Agent Session Identity

Prefer official hooks or integrations where possible.

Every terminal can receive environment variables such as:

```text
MYTERM_PIPE
MYTERM_PANE_ID
MYTERM_WORKSPACE_ID
```

An agent integration can report:

```json
{
  "paneId": "p17",
  "agent": "codex",
  "sessionId": "019f..."
}
```

This gives a precise mapping:

```text
Workspace
    ↓
Tab
    ↓
Pane
    ↓
Agent
    ↓
Agent Session
```

---

# 17. Agent Resume

If the machine restarts:

```text
Restore workspace
   ↓
Restore tab
   ↓
Restore pane
   ↓
Detect saved agent session
   ↓
Build native resume command
```

Examples:

```text
Codex
    codex resume <session-id>

Claude
    claude --resume <session-id>

Cursor Agent
    cursor-agent --resume <session-id>

OpenCode
    opencode --session <session-id>

Hermes
    hermes --resume <session-id>
```

Preserve any original command-line flags where appropriate.

---

# 18. Nested-Agent Ownership

Do not allow a sub-agent to overwrite the main pane's session identity.

Track:

```text
paneId
agentRootPid
processGeneration
sessionId
```

Example:

```text
Pane P17

Main agent:
    codex.exe PID 8724

Sub-agent:
    codex exec PID 9132
```

The sub-agent must not replace the root agent session metadata.

---

# 19. Agent UI

Recommended visual states:

```text
● Working
! Waiting
✓ Done
○ Idle
? Unknown
```

Example workspace navigator:

```text
Synapse
├─ Main
│   ├─ PowerShell
│   └─ ● Codex       working
│
├─ Tests
│   └─ PowerShell
│
└─ Backend
    └─ ! Claude      waiting
```

Agent information can also appear in:

- tab titles
- pane headers
- workspace tree
- status bar
- command palette

---

# 20. Resource-Efficiency Strategy

Resource usage should be treated as an architectural requirement.

## Keep the GUI disposable

The runtime should stay alive without the GUI.

```text
GUI closed:
    WPF controls removed
    terminal renderers removed
    DirectX surfaces removed

Runtime remains:
    PTYs
    shells
    agents
    SQLite
```

This keeps idle resource usage low while preserving terminal state.

---

## Render only visible panes

The GUI should instantiate terminal renderer controls only for the active tab or visible panes.

Hidden workspaces and tabs should remain backend sessions without active render surfaces.

---

## Avoid WebView2 as a baseline dependency

A Tauri/TypeScript client can still be added later, but it should not be required for the primary lightweight application.

---

# 21. Suggested Repository Layout

```text
src/
│
├── TerminalApp/
│   ├── Views/
│   ├── ViewModels/
│   ├── Explorer/
│   ├── Themes/
│   ├── Tabs/
│   ├── Workspaces/
│   ├── Panes/
│   ├── Commands/
│   └── Terminal/
│
├── Terminal.Core/
│   ├── Workspace.cs
│   ├── Tab.cs
│   ├── Pane.cs
│   ├── LayoutNode.cs
│   ├── Shells.cs
│   └── ProtocolModels.cs
│
├── mux-runtime/
│   ├── src/
│   │   ├── pty/
│   │   ├── ipc/
│   │   ├── workspace/
│   │   ├── session/
│   │   ├── persistence/
│   │   ├── process/
│   │   └── agents/
│   └── Cargo.toml
│
└── protocol/
    ├── commands
    ├── events
    └── schemas
```

---

# 22. Development Phases

## Phase 1 — Basic Terminal

Build:

- WPF application
- Microsoft.Terminal.Wpf integration
- single ConPTY terminal
- detected shell profiles for PowerShell 7, Windows PowerShell, Git Bash, and Command Prompt

Goal:

```text
Open app
    ↓
Selected terminal shell works
```

---

## Phase 2 — Pane Engine

Implement:

- binary split tree
- horizontal split
- vertical split
- resize
- close
- directional focus

---

## Phase 3 — Tabs

Implement:

- create tab
- close tab
- rename tab
- reorder tabs
- active-tab handling
- Windows Terminal shortcuts

---

## Phase 4 — Workspaces

Implement:

- workspace creation
- root directories
- workspace switching
- workspace rename
- tab ownership
- new tabs use workspace root

---

## Phase 5 — Persistence

Add:

- SQLite
- saved workspaces
- saved tabs
- saved pane trees
- saved launch commands
- saved CWD
- settings

At this point, restart should reconstruct the application layout.

---

## Phase 6 — Separate Persistent Runtime

Move ConPTY ownership from the GUI into:

```text
mux-runtime.exe
```

Add:

- named-pipe server
- attach
- detach
- resize
- terminal input/output streaming
- GUI-independent process lifetime

This is the architectural milestone where closing the GUI no longer kills terminal sessions.

---

## Phase 7 — Agent Detection

Implement initial process-based detection for:

- Codex
- Claude
- OpenCode
- Cursor
- Hermes

Add visual agent markers.

---

## Phase 8 — Native Agent Session Integration

Add:

- session ID capture
- agent hooks
- session persistence
- resume command generation
- nested-agent ownership

---

## Phase 9 — CLI

Build a lightweight CLI against the same runtime protocol.

Examples:

```text
myterm workspace list

myterm workspace switch Synapse

myterm tab create

myterm pane split --right

myterm agent list
```

---

## Phase 10 — Optional Clients

Because the runtime protocol is independent from the GUI, additional clients can be added later.

Potential clients:

```text
C# WPF GUI
Rust CLI
TypeScript SDK
Tauri/Web UI
VS Code extension
Agent/MCP integration
```

The primary lightweight Windows client remains WPF. The existing public open-source repository remains `tinkershell`, while the application is presented as **Paneacea**.

---

# 23. Final Technology Choice

## Backend

```text
Rust
Tokio
portable-pty / direct ConPTY
windows-rs
SQLite
Named Pipes
```

## GUI

```text
C#
.NET
WPF
Microsoft.Terminal.Wpf
```

## Architecture

```text
GUI client
   │
Named Pipe
   │
persistent Rust mux daemon
   │
ConPTY
   │
shell / agent
```

## Design references

```text
tmux
    client/server architecture

abduco
    session ownership and detach/reattach

dvtm
    presentation/session separation

Herdr
    workspaces and agent model

RMUX
    Rust Windows mux daemon

Windows Terminal
    rendering, panes, keymaps and UX

WezTerm
    terminal/mux architecture reference
```

---

# 24. Final Recommendation

Build **Paneacea** as a **native lightweight Windows client over a persistent Rust terminal daemon**.

The primary design principle should be:

> The GUI displays terminal sessions; it does not own them.

This enables:

- low idle resource usage
- true detach/reattach behavior
- persistent workspaces
- resilient agent sessions
- Windows Terminal-quality rendering
- future CLI and automation clients
- clean separation between runtime and presentation

The recommended stack is therefore:

```text
Rust runtime
+
C# / WPF GUI
+
Microsoft.Terminal.Wpf
+
ConPTY
+
SQLite
+
Named Pipes
```

The WPF presentation should use the VS Code-inspired UI language and theme tokens defined above. This changes the shell chrome, Explorer, command palette, and visual resources only; it does not require a stack change or a browser runtime.

TypeScript remains useful as an optional future SDK or alternate frontend, but it should not be required by the primary lightweight Windows application.
