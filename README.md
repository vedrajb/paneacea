# Paneacea

A Windows terminal workspace app with a native WPF interface and a persistent Rust runtime. This is the **MVP** of the [application plan](doc/plan.md).

The runtime owns the shells. Closing the app detaches the interface; reopening it reconnects to the same running sessions.

## MVP features

- PowerShell 7, Windows PowerShell, Git Bash, and Command Prompt terminals rendered by `Microsoft.Terminal.Wpf` when detected.
- Create, switch, and rename tabs; close individual panes or entire tabs.
- Nested horizontal and vertical splits, draggable dividers, directional focus, and keyboard resizing.
- Create, switch, rename, and close workspaces with their own root directories.
- New tabs and split panes start in their workspace root.
- SQLite persistence for workspace names, roots, tabs, layouts, active selections, and complete launch commands.
- A Settings panel that detects supported shells and selects the default for new tabs and split panes.
- Restore layouts and relaunch shells after a runtime restart.
- Named-pipe input/output streaming and live session reattachment.
- Searchable command palette and a small protocol CLI for development.

## Requirements

- Windows 10 version 1809 or later, or Windows 11, x64.
- .NET 9 SDK (and the .NET 9 Desktop Runtime on a machine running a framework-dependent build).
- Stable Rust with the `x86_64-pc-windows-msvc` toolchain.
- Visual Studio Build Tools with **Desktop development with C++** and a Windows SDK, for Rust linking and bundled SQLite.
- Internet access on the first build to restore NuGet and crates.io dependencies.

The first launch prefers PowerShell 7 (`pwsh`), then Windows PowerShell (`powershell`), Git Bash, and Command Prompt. Open **Settings** from the activity bar or Command Palette to choose another detected shell. The selection applies to new tabs and split panes; existing sessions keep their saved launch command. The renderer is distributed through the pinned [`CI.Microsoft.Terminal.Wpf` package](https://www.nuget.org/packages/CI.Microsoft.Terminal.Wpf/1.25.260303002), a community repackaging of Microsoft's terminal control. This is not an official Microsoft application.

## Build and run

Run from the repository directory in PowerShell:

```powershell
./scripts/build.ps1
& ./src/TerminalApp/bin/Debug/net9.0-windows/win-x64/Paneacea.App.exe
```

The build script builds both components and places `mux-runtime.exe` and `tinkershell.exe` beside the GUI. The app starts the runtime automatically if it is not running. The first launch creates a Default workspace rooted at the launch working directory.

For an optimized build:

```powershell
./scripts/build.ps1 -Configuration Release
& ./src/TerminalApp/bin/Release/net9.0-windows/win-x64/Paneacea.App.exe
```

Keep the entire output directory together: the renderer requires its managed and native libraries. The GUI executable is `Paneacea.App.exe`; the developer CLI remains `tinkershell.exe`. Separate filenames avoid Windows' case-insensitive filename collision.

## Using the app

The WPF shell uses a dark, VS Code-inspired presentation: an activity bar, Explorer workspace list, terminal tabs, pane headers, command palette, and status bar. Use the Explorer to switch projects. Choose **Open Folder**, enter a workspace name, and provide an existing absolute directory. The workspace-root dialog includes **Browse…**, which opens the native Windows folder picker and writes the selected folder back into the path field. Use **+ New Tab**, **Split Right**, or **Split Down** to arrange terminals. Drag a divider to resize panes, and click a pane's header to focus it. Right-click a tab to rename or close it.

**Command Palette** opens the searchable palette, using VS Code-style command names such as `Terminal: Split Right` and `Workspace: Change Workspace Folder`. Changing a workspace root affects future terminals; it does not change a running shell's directory.

Open **Preferences: Open Settings** to view the detected shell paths and choose **Default shell**. Paneacea stores the selected executable and arguments in the runtime settings record so new terminals use the same profile after restart.

| Shortcut | Action |
| --- | --- |
| Ctrl+Shift+T | New tab |
| Ctrl+Shift+W | Close focused pane |
| Ctrl+Shift+P | Command palette |
| Alt+Shift+D | Automatic split direction |
| Alt+Shift+Minus | Split down |
| Alt+Shift+Plus | Split right |
| Alt+Arrow | Directional pane focus |
| Alt+Shift+Arrow | Resize nearest split in that direction |

Closing a pane, tab, or workspace terminates its shells and removes the corresponding saved layout. Closing the main window preserves sessions. If a shell exits, its pane remains visible until you close it or open a new tab.

## Persistence and runtime

By default, Paneacea data lives in `%LOCALAPPDATA%\Paneacea\paneacea.db`, with SQLite WAL companion files. The per-user pipe is `paneacea-<sanitized username>`, with access restricted to the owning user and remote clients rejected. The renamed app starts with a fresh session store; previous sessions are not restored automatically.

To keep development data inside this repository and isolate it from other instances:

```powershell
$env:PANEACEA_DATA = Join-Path $PWD '.data/dev'
$env:PANEACEA_PIPE = 'paneacea-dev'
& ./src/TerminalApp/bin/Debug/net9.0-windows/win-x64/Paneacea.App.exe
```

You can also start the runtime directly:

```powershell
./target/debug/mux-runtime.exe --pipe paneacea-dev --data .data/dev
```

Use one runtime per data directory. To shut it down, first close its workspaces if you want to end and forget their sessions, then end that runtime process. Ending the runtime with saved workspaces still present causes those shells to be relaunched next time. The runtime does not automatically stop when the final window closes.

Two forms of persistence have different guarantees:

- **GUI detach:** existing processes and their in-memory state survive. Reattachment reconstructs the current screen from a bounded runtime terminal model.
- **Runtime restart or reboot:** saved layouts and launch commands are restored. Shell variables, running jobs, output history, and agent conversations are not restored.

## Tests

```powershell
./scripts/test.ps1
```

The Rust tests cover layout edits, malformed IPC frames, SQLite round trips, terminal query handling, shell title mapping, and an integration test using real PowerShell/ConPTY sessions. The integration test exercises default-shell settings, detached output, stable process identity, nested splits, runtime restart, launch flags, and close operations. C# tests cover layout geometry, protocol handling, and shell discovery. An offscreen WPF smoke test loads the native renderer, opens and closes Settings, and exercises terminal input/output, splits, resizing, focus, tabs, reconnect, and window closure. It requires a Windows desktop session. Tests use unique pipe names and leave their databases under ignored `.data` directories for inspection.

## Developer CLI

With the runtime running:

```powershell
./target/debug/tinkershell.exe workspace.list
./target/debug/tinkershell.exe --pipe paneacea-dev state.get
```

The CLI accepts a protocol method and an optional JSON object. See [doc/protocol.md](doc/protocol.md) for request fields and examples. This MVP CLI is a development tool; the friendlier `tinkershell workspace list` syntax belongs to a later phase.

## Architecture

```text
Paneacea.App.exe (WPF + Microsoft.Terminal.Wpf)
                  |
          per-user named pipe
                  |
       mux-runtime.exe (Rust + Tokio)
            |                 |
      portable-pty          SQLite
          ConPTY
            |
        pwsh / Windows PowerShell / Git Bash / cmd
```

`src/Terminal.Core` contains C# protocol models, the pipe client, layout geometry, and Windows shell discovery. `src/TerminalApp` contains the WPF client. `src/mux-runtime` contains the runtime, SQLite store, split tree, terminal model, and developer CLI. The GUI creates render controls only for the selected tab; hidden panes remain runtime sessions.

## MVP limits and next steps

- Agent detection, hooks, session resume, and agent status indicators are deferred.
- Automatic current-directory tracking, terminal-driven tab titles, advanced profile editing, tab reordering, and imported keybindings are deferred. Restart currently uses each pane's saved launch directory.
- Live output scrolls normally, but reattachment restores the current screen, not the complete scrollback. The runtime's VT model does not support every Windows Terminal extension; advanced terminal graphics and uncommon query sequences are not guaranteed.
- Use one GUI client per runtime for now. Layout updates from other clients are not pushed automatically; **Terminal.Reconnect** refreshes the view.
- A slow output client is disconnected to bound memory. Use **Terminal.Reconnect** to attach again. After a runtime failure, relaunch the app to start it again.
- Resource usage has not been benchmarked. Installers, auto-update, optional clients, and a public release license are outside this MVP.

The full roadmap remains in [doc/plan.md](doc/plan.md).
