# Paneacea — Updated Terminal Workspace Plan

## 1. Project Positioning

Paneacea is a personal open-source Windows-first terminal workspace application intended for public use.

```text
GitHub repository / project slug:  paneacea
Product / display name:             Paneacea
CLI / executable:                   paneacea
Persistent runtime:                 paneacea-runtime
```

Core goals:

- tabs
- horizontal/vertical split panes
- pane resizing and focus navigation
- tab rename/reorder
- persistent workspaces
- workspace-specific root directories
- persistent terminal sessions
- Windows Terminal-style shortcuts
- saved tab/pane layouts
- AI-agent detection
- AI-agent session resume where supported
- VS Code-inspired UI
- low idle CPU and controlled memory usage

---

## 2. Updated Stack

### Frontend

Use:

```text
Tauri 2
Svelte
TypeScript
xterm.js
WebView2 on Windows
```

Frontend responsibilities:

- main application window
- activity bar and workspace Explorer
- tabs
- recursive pane layout
- splitters
- command palette
- context menus
- keybinding manager
- settings
- theme system
- agent indicators
- xterm.js terminal rendering
- attach/detach of visible terminal views

### Backend

Keep the persistent runtime in Rust:

```text
Rust
├── Tokio
├── portable-pty or direct ConPTY
├── windows-rs
├── SQLite
├── Named Pipes / local IPC
└── Agent adapters
```

Backend responsibilities:

- own all PTY/ConPTY sessions
- keep sessions alive when the GUI closes
- track processes and agents
- manage runtime session identity
- persist workspace/session metadata
- stream terminal input/output
- restore workspaces
- resume supported agent sessions

---

## 3. Why the Frontend Changed

The previous WPF design made shortcut handling unnecessarily difficult because input could be affected by:

```text
PreviewKeyDown
InputBindings
CommandBindings
Menu accelerators
Focus routing
HwndHost
Terminal control
```

The new design uses xterm.js's terminal-focused keyboard interception.

The central rule is:

> **Paneacea intercepts only explicitly registered application shortcuts. Everything else belongs to the terminal.**

Conceptually:

```text
KeyboardEvent
     │
     ▼
Registered Paneacea shortcut?
     │
   ┌─┴─┐
  yes  no
   │    │
   │    ▼
   │  xterm.js
   │    │
   │    ▼
   │ terminal input encoding
   │    │
   │    ▼
   │ Rust runtime
   │    │
   │    ▼
   │  ConPTY
   │
   ▼
Paneacea action
```

Example:

```ts
terminal.attachCustomKeyEventHandler((event) => {
    const action = shortcutManager.resolve(event);

    if (action) {
        actionRegistry.execute(action);
        return false;
    }

    return true;
});
```

Do not maintain a large allow-list of terminal keys. The app should know only the shortcuts it owns.

This avoids breaking terminal applications such as:

- Vim
- Emacs
- tmux
- shells
- REPLs
- Codex
- Claude
- interactive TUIs

---

## 4. High-Level Architecture

```text
                 Paneacea.exe
                    Tauri 2
                       │
              Svelte / TypeScript
                       │
        ┌──────────────┼──────────────┐
        │              │              │
    Workspaces        Tabs        Pane Tree
        │              │              │
        └──────────────┴──────┬───────┘
                              │
                           xterm.js
                              │
                        streaming IPC
                              │
                              ▼
                    paneacea-runtime.exe
                           Rust
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
      Workspaces             PTYs              Agents
          │                   │                   │
        SQLite            ConPTY/PTY        Codex/Claude
                              │
                    ┌─────────┼─────────┐
                  pwsh      bash      codex
```

Primary architectural rule:

> **The GUI displays terminal sessions; it does not own them.**

This allows users to close and reopen the GUI without terminating shells or agents.

---

## 5. VS Code-Inspired UI

Use a VS Code-inspired visual language without copying its branding.

```text
┌──────┬──────────────────────────────────────────────────────┐
│      │ title bar / menu                                      │
│      ├──────────────────────────────────────────────────────┤
│      │ terminal tabs                                         │
│ Act. ├──────────────────────────────────────────────────────┤
│ bar  │                                                       │
│      │                 terminal pane layout                  │
│      │                                                       │
│      ├──────────────────────────────────────────────────────┤
│      │ status bar                                            │
└──────┴──────────────────────────────────────────────────────┘
```

### Activity Bar / Explorer

Show:

```text
Workspace
├── Tab
│   ├── Pane
│   └── Agent status
└── Tab
```

### Main area

- editor-style terminal tabs
- recursive split-pane tree
- hover-only pane controls
- drag resize
- terminal-first layout

### Status bar

Show useful state such as:

```text
workspace
root folder
shell
active pane
runtime connection
agent status
```

### Command palette

Use command names such as:

```text
Terminal: New Tab
Terminal: Split Right
Terminal: Split Down
Terminal: Close Pane
Workspace: New Workspace
Workspace: Open Folder
Workspace: Switch Workspace
Preferences: Open Settings
Paneacea: Reconnect Runtime
```

---

## 6. Resource-Efficiency Strategy

Tauri is lighter than Electron because it uses the system WebView, but WebView2 still adds baseline memory overhead compared with WPF.

Treat resource efficiency as an architectural requirement.

### 6.1 One WebView per app window

Do:

```text
One Tauri window
└── one WebView2
    ├── sidebar
    ├── tabs
    ├── xterm pane 1
    ├── xterm pane 2
    └── xterm pane 3
```

Do not create a WebView per pane.

### 6.2 Mount only visible terminals

Example:

```text
Runtime:
    15 live PTYs

Frontend:
    3 visible xterm.js instances
```

Hidden tabs/workspaces remain backend sessions without active renderers.

### 6.3 Bound scrollback

Start with:

```text
terminal.scrollback = 10000
```

Make it configurable.

### 6.4 Stream terminal I/O

Do not RPC every keystroke.

Prefer:

```text
xterm.js
   │
   │ input stream
   ▼
Rust runtime
   │
   ▼
ConPTY
```

and:

```text
ConPTY
   │
   │ output stream
   ▼
Rust runtime
   │
   ▼
xterm.js
```

Batch high-frequency output where appropriate.

### 6.5 Dispose hidden WebGL renderers

If the WebGL addon is used, mount it only for visible xterm instances and dispose it cleanly when panes become hidden.

---

## 7. Frontend Components

Suggested Svelte component tree:

```text
App
├── TitleBar
├── ActivityBar
├── Explorer
│   └── WorkspaceTree
├── TerminalTabs
├── PaneTree
│   ├── SplitNode
│   └── TerminalPane
│       └── XtermView
├── CommandPalette
├── StatusBar
└── Settings
```

Likely xterm.js addons:

```text
@xterm/addon-fit
@xterm/addon-search
@xterm/addon-web-links
@xterm/addon-webgl
```

Optional later:

```text
@xterm/addon-serialize
@xterm/addon-unicode11
```

---

## 8. Core Data Model

### Workspace

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

The workspace root is the default working directory for new tabs and panes unless explicitly overridden.

### Tab

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

```text
displayTitle = manualTitle ?? terminalTitle
```

### Pane

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

Persist the complete launch command and arguments.

---

## 9. Pane Layout Model

Use a recursive binary split tree.

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
- deterministic persistence
- straightforward recursive Svelte rendering

---

## 10. Persistence

Use SQLite in the Rust runtime.

Suggested tables:

```text
workspaces
tabs
panes
terminal_sessions
agent_sessions
settings
```

Store recursive pane layouts as JSON.

Two persistence modes must remain distinct.

### Live persistence

```text
GUI closes
   ↓
Tauri/WebView2 exits
   ↓
runtime stays alive
   ↓
ConPTY stays alive
   ↓
shell/agent stays alive
```

### Restart persistence

After reboot/runtime restart:

- restore workspaces
- restore tabs
- restore pane trees
- restore launch commands
- restore CWD metadata
- resume supported agent sessions

---

## 11. IPC Protocol

Keep the protocol frontend-independent.

Suggested commands:

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
tab.move

pane.create
pane.split
pane.close
pane.resize
pane.focus
pane.sendInput

terminal.attach
terminal.detach
terminal.resize
terminal.requestSnapshot

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
pane.cwdChanged

terminal.output
terminal.titleChanged
terminal.exited

agent.detected
agent.statusChanged
agent.sessionChanged
```

High-frequency terminal input/output should use a streaming path rather than ordinary low-frequency command RPC.

---

## 12. Keybinding Architecture

Represent application shortcuts as named actions:

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

Workspace.New
Workspace.Close
Workspace.Next
Workspace.Previous
Workspace.Switch
Workspace.Rename
Workspace.ChangeRoot

Paneacea.CommandPalette
Paneacea.Settings
```

Ship Windows Terminal-compatible defaults where practical:

```text
Alt+Shift+D          split pane
Alt+Shift+-          split down
Alt+Shift++          split right
Alt+Arrow            move focus
Alt+Shift+Arrow      resize pane
Ctrl+Shift+W         close pane
```

Later, optionally import relevant bindings from Windows Terminal `settings.json`.

---

## 13. Shell Profiles

Detect:

```text
pwsh       PowerShell 7
powershell Windows PowerShell 5.1
git-bash   Git Bash for Windows
cmd        Command Prompt
wsl        WSL distributions
```

Persist:

```text
settings.defaultShell
```

including:

- stable profile ID
- executable
- arguments
- availability
- optional icon metadata

Changing the default shell must not affect already-running panes.

---

## 14. Agent Detection

Inspect process trees.

Examples:

```text
pwsh.exe
 └── codex.exe
```

```text
pwsh.exe
 └── node.exe
      └── claude.exe
```

Store:

```text
pane.agent.type
pane.agent.state
pane.agent.sessionId
pane.agent.rootPid
```

Useful backend mechanisms:

- ToolHelp
- Job Objects
- Windows process APIs
- windows-rs

---

## 15. Agent Adapter Interface

Keep agent-specific logic isolated:

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

## 16. Agent Session Identity and Resume

Pass Paneacea identity into terminal environments:

```text
PANEACEA_PIPE
PANEACEA_PANE_ID
PANEACEA_WORKSPACE_ID
```

An integration can report:

```json
{
  "paneId": "p17",
  "agent": "codex",
  "sessionId": "019f..."
}
```

On restoration:

```text
Restore workspace
   ↓
Restore tab
   ↓
Restore pane
   ↓
Load saved agent metadata
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

Preserve original launch flags where appropriate.

---

## 17. Nested-Agent Ownership

Do not let child/sub-agent processes overwrite the main pane's persistent agent identity.

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

Main:
    codex.exe PID 8724

Sub-agent:
    codex exec PID 9132
```

The sub-agent must not automatically replace the root agent session.

---

## 18. Agent UI

Recommended states:

```text
● Working
! Waiting
✓ Done
○ Idle
? Unknown
```

Example Explorer:

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

Agent state may appear in:

- Explorer
- tab title
- pane header
- status bar
- command palette
- notifications

---

## 19. Repository Layout

```text
src/
│
├── app/
│   ├── src/
│   │   ├── components/
│   │   │   ├── activity-bar/
│   │   │   ├── explorer/
│   │   │   ├── tabs/
│   │   │   ├── panes/
│   │   │   ├── terminal/
│   │   │   ├── command-palette/
│   │   │   └── status-bar/
│   │   ├── stores/
│   │   ├── shortcuts/
│   │   ├── themes/
│   │   ├── services/
│   │   ├── protocol/
│   │   └── settings/
│   ├── src-tauri/
│   │   ├── src/
│   │   ├── capabilities/
│   │   └── tauri.conf.json
│   ├── package.json
│   └── vite.config.ts
│
├── paneacea-runtime/
│   ├── src/
│   │   ├── pty/
│   │   ├── ipc/
│   │   ├── workspace/
│   │   ├── session/
│   │   ├── persistence/
│   │   ├── process/
│   │   ├── terminal/
│   │   └── agents/
│   └── Cargo.toml
│
├── paneacea-cli/
│   ├── src/
│   └── Cargo.toml
│
└── protocol/
    ├── schemas/
    ├── commands/
    └── events/
```

---

## 20. Development Phases

### Phase 1 — Tauri Terminal MVP

Build:

- Tauri 2 app
- Svelte UI
- xterm.js
- Rust PTY connection
- PowerShell support
- terminal resize
- terminal I/O

Acceptance criterion:

> Terminal keyboard input and common terminal shortcuts behave correctly.

### Phase 2 — Shortcut Engine

Implement:

- central action registry
- shortcut normalization
- xterm custom-key handler
- pass-through for unknown keys
- configurable bindings

Explicitly test:

```text
Ctrl+C
Ctrl+Z
Ctrl+R
Ctrl+W
Alt combinations
Shift+Tab
function keys
Vim
tmux
Codex
Claude
```

### Phase 3 — Pane Engine

Implement:

- recursive split tree
- horizontal/vertical split
- drag resize
- keyboard resize
- focus navigation
- close pane
- visible-xterm lifecycle

### Phase 4 — Tabs

Implement:

- create
- close
- rename
- reorder
- active tab
- per-tab pane tree
- Windows Terminal-style shortcuts

### Phase 5 — Workspaces

Implement:

- create workspace
- root directory
- switch workspace
- rename workspace
- Explorer
- workspace tab ownership
- new tabs use workspace root

### Phase 6 — Persistence

Add SQLite-backed:

- workspaces
- tabs
- pane layouts
- launch commands
- CWD metadata
- settings

### Phase 7 — Persistent Runtime

Move all PTY ownership into:

```text
paneacea-runtime.exe
```

Add:

- named-pipe/local IPC server
- attach/detach
- terminal streaming
- resize
- GUI-independent process lifetime

### Phase 8 — Resource Optimization

Measure and optimize:

```text
idle RAM
idle CPU
1 visible terminal
4 visible terminals
10 hidden sessions
heavy terminal output
workspace switching
```

Implement:

- only-visible xterm mounting
- bounded scrollback
- output batching
- background-tab detachment
- WebGL cleanup

### Phase 9 — Agent Detection

Add initial support for:

- Codex
- Claude
- OpenCode
- Cursor
- Hermes

### Phase 10 — Native Agent Session Integration

Add:

- session ID capture
- hooks
- resume command generation
- nested-agent ownership

### Phase 11 — CLI

Examples:

```text
paneacea workspace list
paneacea workspace switch Synapse
paneacea tab create
paneacea pane split --right
paneacea agent list
```

### Phase 12 — Optional Integrations

Potential later clients/integrations:

```text
VS Code extension
MCP server
TypeScript SDK
Rust SDK
remote client
web client
automation hooks
```

---

## 21. Architecture References

### tmux
Borrow client/server persistence.

### abduco
Borrow session ownership independent of presentation.

### dvtm
Borrow layout/presentation separation.

### Herdr
Borrow workspaces, persistent runtime, agent state, attach/detach, and session restoration.

### RMUX
Borrow Windows-friendly Rust daemon architecture.

### Windows Terminal
Borrow keybinding vocabulary, pane/tab UX, shell profiles, and command conventions.

### xterm.js
Use for terminal rendering, terminal keyboard behavior, VT handling, selection, search, links, and optional WebGL acceleration.

### VS Code
Borrow the UI language: activity bar, Explorer, editor-style tabs, command palette, compact status bar.

---

## 22. Final Technology Choice

### Frontend

```text
Tauri 2
Svelte
TypeScript
xterm.js
WebView2
```

### Backend

```text
Rust
Tokio
portable-pty / direct ConPTY
windows-rs
SQLite
Named Pipes / local IPC
```

### Architecture

```text
Tauri/Svelte GUI
      │
   xterm.js
      │
streaming IPC
      │
persistent Rust mux daemon
      │
   ConPTY
      │
shell / agent
```

---

## 23. Final Recommendation

Build Paneacea as:

> **A Tauri/Svelte terminal-workspace client over a persistent Rust multiplexer daemon.**

The original backend architecture remains unchanged:

- Rust persistent runtime
- ConPTY owned outside the GUI
- SQLite persistence
- workspaces/tabs/panes
- agent integration
- attach/detach
- native agent resume where supported

The frontend changes from:

```text
C# / WPF
Microsoft.Terminal.Wpf
```

to:

```text
Tauri 2
Svelte
TypeScript
xterm.js
```

The main reasons are:

1. simpler and more reliable terminal shortcut handling
2. faster development of a VS Code-style workspace UI
3. cleaner split between application shortcuts and terminal input
4. easier long-term UI iteration

The main cost is WebView2 memory overhead.

Mitigate it by:

- one WebView per window
- render only visible terminals
- keep hidden sessions backend-only
- bound browser-side scrollback
- stream terminal I/O
- carefully manage WebGL terminal lifecycles

The defining keyboard invariant is:

> **Paneacea intercepts only its own registered shortcuts. Every other key belongs to the terminal.**
