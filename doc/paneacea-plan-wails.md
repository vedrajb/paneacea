# Paneacea — Full Wails Architecture Plan

## 1. Project Positioning

Paneacea is a **personal open-source project intended for public use**.

Naming convention:

```text
GitHub repository / project slug:  paneacea
Product / display name:             Paneacea
CLI / executable:                   paneacea
Persistent runtime:                 paneacea-runtime
```

Paneacea is a Windows-first, terminal-first developer workspace built around persistent sessions.

Core differentiators:

- persistent workspaces
- tabs and recursive split panes
- workspace-specific root folders
- Windows Terminal-style keybindings
- persistent terminal processes independent of the GUI
- AI-agent detection
- AI-agent session resume where supported
- VS Code-inspired UI
- lightweight detached runtime

---

## 2. Product Goals

Build a terminal workspace application with:

- tabs
- horizontal and vertical split panes
- pane resizing
- directional pane focus
- tab rename/reorder
- persistent workspaces
- workspace root folders
- workspace navigation
- saved layout state
- saved launch commands
- Windows Terminal-compatible shortcuts
- persistent terminal sessions
- AI-agent detection
- agent state indicators
- agent session restoration
- VS Code-inspired presentation
- low idle CPU
- controlled memory usage

Architectural references:

- tmux
- abduco
- dvtm
- Herdr
- RMUX
- Windows Terminal
- WezTerm
- xterm.js
- VS Code

---

## 3. Recommended Stack

### Frontend / Desktop Shell

Use:

```text
Wails
Svelte
TypeScript
xterm.js
WebView2 on Windows
```

Frontend responsibilities:

- application window
- activity bar
- workspace Explorer
- terminal tabs
- recursive pane layout
- splitters
- command palette
- context menus
- settings UI
- theme system
- keybinding manager
- xterm.js terminal views
- agent indicators
- status bar

### Persistent Runtime

Use **Go**.

Recommended components:

```text
Go
├── goroutines/channels
├── ConPTY integration
├── Windows process APIs
├── SQLite
├── Named Pipes / local IPC
├── process-tree monitoring
└── Agent adapters
```

Runtime responsibilities:

- own all PTY/ConPTY sessions
- remain alive when the GUI closes
- track session/process state
- persist workspace metadata
- expose IPC to GUI and CLI
- stream terminal input/output
- restore saved layouts
- resume supported agent sessions

---

## 4. Process Architecture

Do **not** let the Wails GUI own long-lived PTYs.

Use two processes:

```text
paneacea.exe
    Wails GUI
    Svelte
    TypeScript
    xterm.js

paneacea-runtime.exe
    Go daemon
    PTYs
    SQLite
    agents
```

Architecture:

```text
                 paneacea.exe
                    Wails
                      │
               Svelte / TS
                      │
      ┌───────────────┼────────────────┐
      │               │                │
  Workspaces         Tabs          Pane Tree
      │               │                │
      └───────────────┴──────┬─────────┘
                             │
                          xterm.js
                             │
                       local IPC/stream
                             │
                             ▼
                 paneacea-runtime.exe
                          Go
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

Primary invariant:

> **The GUI displays terminal sessions; it does not own them.**

Closing Paneacea's GUI must not terminate shells or agents.

---

## 5. Why Wails

Wails preserves the frontend model best suited to Paneacea:

```text
Svelte + TypeScript + xterm.js
```

while letting the native side be written in Go.

Benefits:

- simpler backend concurrency than Rust
- straightforward process management
- strong standard library
- good fit for many PTY sessions
- generated frontend bindings
- easy code sharing between GUI helpers, daemon, and CLI
- one native backend language

Tradeoffs:

- WebView2 baseline memory
- Go garbage collector
- high-frequency terminal output must be batched carefully
- Wails transport must be validated under heavy terminal load

Proceed directly with the Wails implementation. Validate keyboard, streaming, lifecycle, and resource behavior during implementation.

---

## 6. Keyboard Architecture

This is a core reason for choosing xterm.js over the previous WPF terminal frontend.

Rule:

> **Paneacea intercepts only explicitly registered application shortcuts. Every other key belongs to xterm.js and the terminal.**

Flow:

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
   │ terminal bytes
   │    │
   │    ▼
   │ runtime
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

Do not build a large allow-list of terminal keys. The terminal is the default key owner.

### Selection-aware Ctrl+C

The goal is to make Ctrl+C copy selected terminal text and interrupt the running process when nothing is selected.

The next step is to implement selection-aware handling:

- when xterm.js has a selection, copy the selected terminal text and do not send Ctrl+C to ConPTY
- when there is no selection, pass Ctrl+C through so the terminal can interrupt the running process

Verify both paths in the keyboard test matrix, including inside interactive terminal applications.

---

## 7. Terminal Streaming

Terminal traffic is a stream, not ordinary application RPC.

Avoid sending every tiny PTY read as a separate frontend event.

Prefer:

```text
PTY reads
   ↓
Go buffer
   ↓
flush on timer OR size threshold
   ↓
frontend transport
   ↓
xterm.write(...)
```

Suggested initial batching target:

```text
4-8 ms
```

Flush earlier when the buffered byte count exceeds a threshold.

Input should likewise use an efficient persistent path rather than an expensive command abstraction for each keystroke.

Add bounded queues and backpressure so a runaway producer cannot exhaust memory.

---

## 8. Resource-Efficiency Strategy

### One WebView per application window

```text
One Wails window
└── one WebView2
    ├── activity bar
    ├── Explorer
    ├── tabs
    ├── xterm pane 1
    ├── xterm pane 2
    └── xterm pane 3
```

Never create one WebView per terminal pane.

### Mount only visible xterm.js instances

Example:

```text
Runtime:
    20 live terminal sessions

GUI:
    3 visible xterm.js instances
```

Hidden tabs and workspaces remain backend-only sessions.

### Bound frontend scrollback

Start with:

```text
terminal.scrollback = 10000
```

Make it configurable.

### Manage WebGL lifecycle carefully

If `@xterm/addon-webgl` is used:

- enable it only for visible terminals
- dispose it with the terminal view
- test repeated create/destroy
- fall back to the default renderer if needed

### Keep the detached runtime lightweight

When the GUI closes:

```text
Wails
Svelte
WebView2
xterm.js
```

all exit.

The runtime remains:

```text
paneacea-runtime.exe
├── PTYs
├── shells
├── agents
├── SQLite
└── IPC endpoint
```

This is the key resource-saving behavior.

---

## 9. VS Code-Inspired UI

Use a VS Code-inspired interface without copying VS Code branding.

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

Explorer model:

```text
Workspace
├── Tab
│   ├── Pane
│   └── Agent
└── Tab
```

Main area:

- terminal tabs
- recursive split panes
- compact headers
- hover-only actions
- drag splitters
- xterm.js terminals

Status bar:

```text
workspace
root folder
shell
active pane
runtime status
agent status
```

Command examples:

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

## 10. Svelte Component Structure

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

## 11. Theme System

Use CSS custom properties.

Example:

```css
:root {
  --editor-bg: #1e1e1e;
  --sidebar-bg: #252526;
  --activity-bg: #333333;
  --panel-bg: #181818;
  --tab-bg: #2d2d2d;
  --tab-active-bg: #1e1e1e;
  --border: #3f3f46;
  --foreground: #cccccc;
  --muted-foreground: #858585;
  --accent: #0078d4;
  --focus: #4fc1ff;
  --error: #f14c4c;
  --warning: #cca700;
  --success: #89d185;
}
```

Terminal colors remain separate xterm.js theme objects.

---

## 12. Core Data Model

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

New tabs use the workspace root by default.

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

Persist the full launch command and arguments.

---

## 13. Pane Layout Model

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
               /                    Pane A        Horizontal
                         /                            Pane B     Pane C
```

Benefits:

- arbitrary nested splits
- simple recursive Svelte rendering
- drag resizing
- keyboard resizing
- directional focus
- exact persistence

---

## 14. Shell Profiles

Detect:

```text
git-bash   Git Bash
pwsh       PowerShell 7
powershell Windows PowerShell 5.1
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

Changing the default does not affect existing sessions.

---

## 15. Persistence

Use SQLite in the Go runtime.

Suggested tables:

```text
workspaces
tabs
panes
terminal_sessions
agent_sessions
settings
```

Store recursive layouts as JSON.

### Live persistence

```text
GUI closes
   ↓
runtime remains
   ↓
ConPTY remains
   ↓
shell/agent remains
```

### Restart persistence

After reboot or daemon restart:

- restore workspaces
- restore tabs
- restore pane trees
- restore launch commands
- restore working directories
- restore agent metadata
- resume supported agent sessions

---

## 16. IPC Protocol

Keep the runtime protocol independent of Wails.

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

High-frequency terminal data uses the dedicated streaming/batched path.

---

## 17. Keybinding Model

Represent application actions explicitly:

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

Later, optionally import compatible bindings from Windows Terminal `settings.json`.

---

## 18. Agent Detection

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

Useful Windows mechanisms:

- ToolHelp
- process APIs
- Job Objects
- WMI only where necessary

---

## 19. Agent Adapter Interface

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

## 20. Agent Session Identity and Resume

Pass Paneacea identity through environment variables:

```text
PANEACEA_PIPE
PANEACEA_PANE_ID
PANEACEA_WORKSPACE_ID
```

Integration example:

```json
{
  "paneId": "p17",
  "agent": "codex",
  "sessionId": "019f..."
}
```

On full restoration:

```text
Restore workspace
   ↓
Restore tab
   ↓
Restore pane
   ↓
Load agent metadata
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

Preserve original launch flags.

---

## 21. Nested-Agent Ownership

Do not let temporary child agents overwrite the pane's root session identity.

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

The sub-agent must not automatically replace the root session.

---

## 22. Agent UI

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

Surface state in:

- Explorer
- tab title
- pane header
- status bar
- command palette
- notifications

---

## 23. Repository Layout

Use one Go repository with separate binaries:

```text
paneacea/
│
├── cmd/
│   ├── paneacea/
│   │   └── Wails GUI entrypoint
│   ├── paneacea-runtime/
│   │   └── persistent daemon
│   └── paneacea-cli/
│       └── CLI
│
├── internal/
│   ├── workspace/
│   ├── tabs/
│   ├── panes/
│   ├── session/
│   ├── terminal/
│   ├── conpty/
│   ├── ipc/
│   ├── persistence/
│   ├── process/
│   ├── agents/
│   └── protocol/
│
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   ├── stores/
│   │   ├── shortcuts/
│   │   ├── themes/
│   │   ├── services/
│   │   └── settings/
│   ├── package.json
│   └── vite.config.ts
│
├── go.mod
└── wails.json
```

---

## 24. Development Phases

### Phase 1 — Product Shell

Build:

- Wails app shell
- Svelte layout
- activity bar
- Explorer placeholder
- terminal tabs placeholder
- status bar
- theme tokens

### Phase 2 — Terminal Core

Implement:

- xterm.js
- Go ConPTY session
- input/output stream
- resize
- title handling
- close/restart handling

### Phase 3 — Shortcut Engine

Implement:

- central action registry
- keybinding normalization
- xterm custom-key handler
- pass-through for unknown keys
- configurable shortcuts

Test:

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

### Phase 4 — Pane Engine

Implement:

- recursive split tree
- horizontal/vertical split
- drag resize
- keyboard resize
- focus navigation
- close pane
- visible-xterm lifecycle

### Phase 5 — Tabs

Implement:

- create
- close
- rename
- reorder
- active tab
- pane tree per tab
- Windows Terminal-style shortcuts

### Phase 6 — Workspaces

Implement:

- create workspace
- root folder
- switch
- rename
- tab ownership
- Explorer
- new tabs use workspace root

### Phase 7 — Persistence

Add SQLite:

- workspaces
- tabs
- layouts
- launch commands
- CWD
- settings

### Phase 8 — Persistent Runtime

Move all PTY ownership into:

```text
paneacea-runtime.exe
```

Implement:

- daemon startup
- named-pipe/local IPC
- attach/detach
- terminal streaming
- resize
- terminal snapshots
- GUI-independent lifetime

### Phase 9 — Resource Optimization

Measure:

```text
idle RAM
idle CPU
1 visible terminal
4 visible terminals
10 hidden sessions
heavy output
workspace switching
long-running session
```

Optimize:

- only-visible xterm instances
- batching
- bounded scrollback
- listener cleanup
- WebGL cleanup
- daemon idle behavior

### Phase 10 — Agent Detection

Implement:

- Codex
- Claude
- OpenCode
- Cursor
- Hermes

### Phase 11 — Agent Session Integration

Add:

- session ID capture
- hooks/integrations
- persisted metadata
- native resume
- nested-agent rules

### Phase 12 — CLI

Examples:

```text
paneacea workspace list
paneacea workspace switch Synapse
paneacea tab create
paneacea pane split --right
paneacea agent list
```

The CLI communicates with the same runtime daemon.

### Phase 13 — Optional Integrations

Potential later additions:

```text
VS Code extension
MCP server
TypeScript SDK
Go SDK
remote client
web client
automation hooks
```

---

## 25. Wails-Specific Risks

### High-frequency frontend transport

Mitigate with:

- batching
- larger chunks
- bounded queues
- backpressure
- profiling under sustained output

### WebView2 memory overhead

Mitigate with:

- one WebView per window
- visible terminals only
- bounded scrollback
- listener/addon disposal
- shells and agents kept out of GUI process

### Go garbage collection

Avoid unnecessary short-lived allocations in the hot terminal path.

Use buffer reuse only where profiling proves it is useful.

### Framework dependency

Keep runtime IPC independent from Wails so the GUI can be replaced later without rewriting:

- PTY management
- persistence
- sessions
- agents
- CLI

---

## 26. Architecture References

### tmux
Borrow client/server persistence.

### abduco
Borrow attach/detach session ownership.

### dvtm
Borrow presentation/session separation.

### Herdr
Borrow workspaces, tabs, panes, background runtime, agent state, and restoration.

### RMUX
Borrow persistent daemon/client architecture.

### Windows Terminal
Borrow keybinding vocabulary, tab/pane UX, profiles, and command conventions.

### xterm.js
Use for rendering, keyboard semantics, VT behavior, selection, search, links, and optional WebGL.

### VS Code
Borrow activity bar, Explorer, editor-style tabs, command palette, and status bar.

---

## 27. Final Technology Choice

### Frontend

```text
Wails
Svelte
TypeScript
xterm.js
WebView2
```

### Runtime

```text
Go
ConPTY
SQLite
Named Pipes / local IPC
process monitoring
agent adapters
```

### Architecture

```text
Wails/Svelte GUI
      │
   xterm.js
      │
streaming IPC
      │
persistent Go runtime
      │
   ConPTY
      │
shell / agent
```

---

## 28. Final Recommendation

Use Wails. Validate terminal streaming and memory stability as part of implementation and release testing.

Final design:

> **A Wails/Svelte terminal workspace client over a persistent Go multiplexer daemon.**

Primary architectural principles:

1. **The GUI does not own terminal processes.**
2. **Only visible terminals have xterm.js renderers.**
3. **Paneacea intercepts only its own registered shortcuts.**
4. **Everything else belongs to the terminal.**
5. **Terminal data uses a streaming/batched transport.**
6. **The runtime protocol stays independent from Wails.**
7. **Workspaces, tabs, panes, sessions, and agent state are persisted by the runtime.**

This preserves the frontend advantages of Svelte/xterm.js while simplifying the native backend compared with the Rust/Tauri plan.
