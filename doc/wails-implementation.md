# Paneacea Wails application

The Go/Wails application is implemented at the repository root. The earlier Rust/WPF app and disposable `wails-prototype` remain separate. Phase 0 has been removed from the architecture plan at the user's request.

## Build and run

Requirements: Windows 10 1809 or later, WebView2 Runtime, Go 1.24.2 or newer, and Node.js 20.19 or newer.

From the repository root:

Double-click `build-wails.bat` to build and launch, or run `build-wails.bat --build-only` to build without opening the app.

```powershell
.\scripts\build-wails.ps1 -Run
```

Or launch an existing build:

```powershell
.\build\bin\paneacea.exe
```

The build script compiles all three binaries together. It uses the workspace-local Go toolchain if present, otherwise Go from PATH. A separately installed Wails CLI is not required.

Keep `paneacea.exe`, `paneacea-runtime.exe`, and `paneacea-cli.exe` in the same directory. The GUI starts the runtime when needed. Closing the GUI leaves terminal processes running. Closing a pane, tab, or workspace terminates its owned sessions.

The runtime stores SQLite metadata under `%APPDATA%\Paneacea\go-runtime\paneacea.db`. Set `PANEACEA_DATA_DIR` before starting the runtime to choose a different directory. This database is separate from the Rust runtime's data. Existing Rust/WPF workspaces are not imported.

## Use

- Create a workspace and choose its root folder, then open a terminal tab.
- Use `Alt+Shift+D` to split automatically, `Alt+Shift++` to split right, or `Alt+Shift+-` to split down.
- Use `Alt+Arrow` to move focus and `Alt+Shift+Arrow` to resize. Splitters also support dragging and arrow keys.
- Double-click a tab to rename it. Drag tabs to reorder them; right-click for tab actions.
- `Ctrl+T` or `Ctrl+Shift+T` opens a tab. `Ctrl+W` closes the entire current tab and all its panes; `Ctrl+Shift+W` closes only the focused pane.
- `Ctrl+Tab` / `Ctrl+Shift+Tab` switch to the next / previous tab. `Ctrl+Alt+Tab` / `Ctrl+Alt+Shift+Tab` switch to the next / previous workspace.
- `Ctrl+N` creates a workspace, `Ctrl+Alt+R` renames it, and Ctrl+backtick focuses the active terminal.
- `Ctrl++` / `Ctrl+-` change terminal font size from 8 to 32, including the main keyboard and numpad keys, with or without Shift. Ctrl+mouse wheel over a terminal also changes font size. The size is saved and applied to every terminal.
- `Ctrl+?` (`Ctrl+Shift+/`) opens keyboard shortcut help with current bindings and customizations. Help is also available in the command palette.
- `Ctrl+Shift+P` opens the command palette. `Ctrl+,` opens settings.
- Ctrl+C copies selected terminal text. With no selection it reaches the terminal as an interrupt.
- Preferences include default shell, font size, scrollback, appearance, and custom keybindings. Existing sessions retain their original launch configuration.

## Runtime and IPC

The Go runtime owns ConPTY, terminal screen emulation, process monitoring, and persistence. The Wails bridge only forwards runtime requests. Only visible tabs mount xterm instances. Output is batched at 6 ms or 64 KiB, retained in a bounded 2 MiB stream per pane, and pulled in acknowledged batches. Slow clients recover from a current screen snapshot instead of accumulating unbounded output queues. Terminal queries are answered by the runtime even with no GUI attached.

The named pipe is scoped to the current Windows user's SID and grants access only to that user. It uses an independent Go protocol, separate from the legacy Rust protocol. Requests are newline-delimited JSON with `id`, `method`, and `params`; responses contain `id`, `ok`, and `result` or `error`. Output data is base64 and carries a sequence number. Initial attachment returns a screen snapshot and subsequent reads continue from that sequence. Client connection closure detaches without terminating the terminal.

Implemented methods: `state.get`, `profiles.list`, `workspace.list/create/rename/setRoot/switch/close`, `tab.create/rename/focus/move/close`, `pane.create/split/focus/resize/close/restart/sendInput`, `terminal.attach/detach/read/resize/requestSnapshot`, `settings.set`, and `agent.list/get/status/register/resume`.

Workspace/tab/pane updates are synchronized through persisted state revisions. The GUI checks state every 1.5 seconds; terminal traffic uses separate persistent connections. SQLite atomically saves the complete versioned workspace aggregate in `application_state`, including recursive layouts, panes, settings, and agent metadata. Mutations that fail persistence are rolled back. Native process state is not transactionally rewindable.

## CLI

```powershell
.\build\bin\paneacea-cli.exe workspace list
.\build\bin\paneacea-cli.exe workspace create Project C:\dev\project
.\build\bin\paneacea-cli.exe workspace switch Project
.\build\bin\paneacea-cli.exe tab create
.\build\bin\paneacea-cli.exe pane split --right
.\build\bin\paneacea-cli.exe agent list
```

Any IPC method can also be sent directly with a JSON parameter object. The console CLI is recommended for scripts because the GUI executable uses the Windows GUI subsystem.

```powershell
.\build\bin\paneacea-cli.exe tab.create '{"workspaceId":"ID","executable":"cmd.exe","arguments":[]}'
```

## Agent integration

Process-tree monitoring recognizes native Codex, Claude, OpenCode, Cursor Agent, Copilot, and Hermes executable names. Process detection shows `unknown` until an integration reports a meaningful state. Detection does not infer “working” or “waiting” from terminal text.

The runtime injects `PANEACEA_PIPE`, `PANEACEA_PANE_ID`, and `PANEACEA_WORKSPACE_ID`. An agent hook or wrapper can register captured metadata:

```powershell
.\build\bin\paneacea-cli.exe agent register '{"type":"claude","state":"working","sessionId":"SESSION-ID","rootPid":1234,"executable":"claude.exe","arguments":["--model","sonnet"]}'
```

The CLI fills the process generation from the root PID. The runtime verifies that PID belongs to the pane and rejects a nested agent replacing a live root identity. Report subsequent states using the same root PID and captured session ID. Launch flags must be supplied by the integration; process-name detection alone cannot capture them or discover a session ID.

Resume uses the native command forms from the plan for Codex, Claude, Cursor, OpenCode, and Hermes. Copilot resume is explicitly unsupported. Resume requires a captured session ID and executable. Restarting the runtime restores launch commands and supported captured agent sessions; shell variables and live OS processes cannot survive a runtime restart or reboot.

## Validation and limits

```powershell
.\scripts\test-wails.ps1
```

Run `npm run test:browser` from `frontend` for the browser interaction suite. It covers terminal rendering, splitting, resizing, tab renaming, settings, and command-palette interaction through a mocked Wails bridge. On Windows, Playwright may need Ctrl+C after reporting successful browser results because its Vite child server does not always shut down cleanly.

Automated tests cover keyboard ownership, recursive layouts, SQLite reopen, mutation rollback, headless PowerShell input/output, environment propagation, detach, resizing, runtime restoration, and bounded output. The Windows GUI still needs manual keyboard/TUI, sustained-output, and long-duration resource validation before release.

Reconnect restores the current terminal screen and bounded recent scrollback. Terminal display history is held in runtime memory and is not stored in SQLite. A full runtime restart restores configuration and starts fresh processes. Shell CWD tracking uses OSC 7 prompt integration for PowerShell and Git Bash; other programs must emit OSC 7 to report directory changes. WSL directory tracking and arbitrary Node/Python agent-wrapper identification require explicit integrations.

Optional extensions, MCP, SDKs, remote clients, and web clients from Phase 13 remain optional future work.
