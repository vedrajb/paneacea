# Paneacea

A Windows terminal workspace app with a native WPF interface and a persistent Rust runtime.

The runtime owns the shells. Closing the app detaches the interface; reopening it reconnects to the same running sessions.

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
& ./portable-release/Paneacea.App.exe
```

The build script builds both components and places the runtime and protocol CLI beside the GUI. The app starts the runtime automatically if it is not running. The first launch creates a Default workspace rooted at the launch working directory.

For an optimized build:

```powershell
./scripts/build.ps1 -Configuration Release
& ./portable-release/Paneacea.App.exe
```

Keep the entire output directory together: the renderer requires its managed and native libraries. The GUI executable is `Paneacea.App.exe`; the output directory also contains the runtime and protocol CLI. This directory is the portable deployment root. Separate filenames avoid Windows' case-insensitive filename collision.

## Using the app

The WPF shell uses a dark, VS Code-inspired presentation: an activity bar, Explorer workspace list, terminal tabs, pane headers, command palette, and status bar. Use the Explorer to switch projects. Choose **Open Folder**, enter a workspace name, and provide an existing absolute directory. The workspace-root dialog includes **Browse…**, which opens the native Windows folder picker and writes the selected folder back into the path field. Use **+ New Tab**, **Split Right**, or **Split Down** to arrange terminals. Drag a divider to resize panes, and click a pane's header to focus it. Right-click a tab to rename or close it.

**Command Palette** opens the searchable palette, using VS Code-style command names such as `Terminal: Split Right` and `Workspace: Change Workspace Folder`. Changing a workspace root affects future terminals; it does not change a running shell's directory. Interactive PowerShell and Git Bash panes report their current folder so it can be restored independently per pane.

Open **Preferences: Open Settings** to view the detected shell paths and choose **Default shell**. Paneacea stores the selected executable and arguments in the runtime settings record so new terminals use the same profile after restart.

| Shortcut | Action |
| --- | --- |
| Ctrl+Tab | Next tab |
| Ctrl+Shift+Tab | Previous tab |
| Ctrl+Alt+Tab | Next workspace |
| Ctrl+Alt+Shift+Tab | Previous workspace |
| Ctrl+Alt+R | Rename workspace |
| Ctrl+T | New tab |
| Ctrl+W | Close current tab |
| Ctrl+N | New workspace |
| Ctrl+\` | Focus terminal pane |
| Ctrl++ | Increase terminal font size |
| Ctrl+- | Decrease terminal font size |
| Ctrl+Mouse Wheel | Change terminal font size |
| Ctrl+? | Keyboard shortcuts popup |
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

By default, the directory containing `Paneacea.App.exe` is the portable data root. `paneacea.db`, its SQLite WAL companion files, and the `settings` table all live directly under that root. Logs are written under `<root>\logs\`. The per-user pipe is `paneacea-<sanitized username>`, with access restricted to the owning user and remote clients rejected. Layout and per-pane state are restored when the app reconnects to the same data directory and user-owned runtime.

To keep development data inside this repository and isolate it from other instances:

```powershell
$env:PANEACEA_DATA = Join-Path $PWD '.data/dev'
$env:PANEACEA_PIPE = 'paneacea-dev'
& ./portable-release/Paneacea.App.exe
```

You can also start the runtime directly:

```powershell
./target/debug/panacea-runtime.exe --pipe paneacea-dev --data .data/dev
```

Use one runtime per data directory. To shut it down, first close its workspaces if you want to end and forget their sessions, then end that runtime process. Ending the runtime with saved workspaces still present causes those shells to be relaunched next time. The runtime does not automatically stop when the final window closes.

Two forms of persistence have different guarantees:

- **GUI detach:** existing processes and their in-memory state survive. Reattachment reconstructs the current screen from a bounded runtime terminal model.
- **Runtime restart or reboot:** saved layouts, split ratios, pane folders, terminal dimensions, and encrypted terminal history are restored per pane. Panes reopen at the top of their restored history; scrollbar positions are not persisted. The shell process is relaunched in the saved folder; shell variables, running jobs, and live full-screen application processes are not resumed.

Saved terminal history is configured in Settings with Off, 500, 2,000, 5,000, 10,000, and 25,000-line presets. History is encrypted with Windows DPAPI for the current Windows user. Off removes persisted history while leaving the current runtime's in-memory terminal behavior available.

When diagnosing restore problems, inspect `logs\paneacea-runtime.log` and `logs\paneacea-app.log` under the portable root (or under the `PANEACEA_DATA` root when that variable is set). Both logs include restore paths, state counts, pane IDs, history-row outcomes, and attach metadata, but never terminal input or saved terminal text. Set `PANEACEA_LOG` to override the application log path.

## Tests

```powershell
./scripts/test.ps1
```

The Rust tests cover layout edits, malformed IPC frames, encrypted SQLite history round trips, terminal query and OSC 7 handling, shell title mapping, and integration tests using real PowerShell/ConPTY and Git Bash sessions. The integration tests exercise per-pane folders and history, live viewport behavior, default-shell settings, detached output, stable process identity, nested splits, runtime restart, launch flags, and close operations. C# tests cover layout geometry, protocol handling, shell discovery, and saved-history settings. An offscreen WPF smoke test loads the native renderer, opens and closes Settings, and exercises terminal input/output, splits, resizing, focus, tabs, reconnect, scrollbar behavior, and window closure. It requires a Windows desktop session. Tests use unique pipe names and leave their databases under ignored `.data` directories for inspection.

## Developer protocol CLI

With the runtime running:

```powershell
./target/debug/paneacea.exe workspace.list
./target/debug/paneacea.exe --pipe paneacea-dev state.get
```

The CLI accepts a protocol method and an optional JSON object. See [doc/protocol.md](doc/protocol.md) for request fields and examples.

## Architecture

```text
Paneacea.App.exe (WPF + Microsoft.Terminal.Wpf)
                  |
          per-user named pipe
                  |
       panacea-runtime.exe (Rust + Tokio)
            |                 |
      portable-pty          SQLite
          ConPTY
            |
        pwsh / Windows PowerShell / Git Bash / cmd
```

`src/Terminal.Core` contains C# protocol models, the pipe client, layout geometry, and Windows shell discovery. `src/TerminalApp` contains the WPF client. `src/paneacea-runtime` contains the runtime, SQLite store, split tree, terminal model, and developer CLI. The runtime binary is `panacea-runtime.exe`. The GUI creates render controls only for the selected tab; hidden panes remain runtime sessions.

The full roadmap remains in [doc/plan.md](doc/plan.md).
