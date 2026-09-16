# Paneacea

A Windows terminal workspace app built with Wails, Svelte, and a persistent Go runtime. The earlier Rust/C# source is archived under `archive/rust-csharp`.

## Requirements

- Windows 10 version 1809 or later, or Windows 11 (x64)
- WebView2 Runtime
- Go 1.24.2 or newer
- Node.js 20.19 or newer
- Internet access on the first dependency installation and build

## Install dependencies

Run from the repository root:

```powershell
npm i
```

The root npm `postinstall` installs the frontend dependencies and downloads the Go modules. To invoke the underlying script explicitly, use:

```powershell
npm run install:all
```

## Build and run

From the repository root, build and launch the application with:

```powershell
.\run.bat
```

Build without launching:

```powershell
.\build.bat
```

Stop all running Paneacea application, runtime, and CLI executables:

```powershell
.\scripts\stop-paneacea.bat
```

The same command is available through npm:

```powershell
npm run stop
```

Both commands default to the `Release` configuration. Pass `Debug` or `Release` as the first argument to select a configuration.

To recreate the portable package from scratch and create a zip archive, run:

```powershell
.\package.bat
```

This recreates `portable-release` and writes `portable-release.zip`.

The PowerShell build script can also be run directly. Omit `-Run` to build without launching:

```powershell
.\scripts\build-wails.ps1 -Configuration Release
.\scripts\build-wails.ps1 -Configuration Release -Run
```

The build writes intermediate executables to `build\bin` and creates the portable package at `portable-release`:

```text
portable-release/
├── paneacea.exe
├── paneacea-runtime.exe
├── paneacea-cli.exe
├── Logs/
├── config.toml
└── pca.cmd
```

`config.toml` is seeded with default user settings and is preserved when the package is rebuilt. Launch the packaged app from a terminal with:

```powershell
.\portable-release\pca.cmd
```

The intermediate build output contains:

- `paneacea.exe` — Wails desktop application
- `paneacea-runtime.exe` — Go runtime
- `paneacea-cli.exe` — developer protocol CLI

Launch the intermediate build directly with:

```powershell
.\build\bin\paneacea.exe
```

Keep all three executables in the same directory. A separately installed Wails CLI is not required. The build script uses the workspace-local Go toolchain when available and otherwise uses Go from `PATH`.

## Usage

After launch, create a workspace, choose its root folder, and open a terminal tab. The first launch detects Git Bash, PowerShell 7, Windows PowerShell, and Command Prompt. Use Settings to choose the default shell, font size, scrollback, appearance, and custom keybindings.

Use the command palette (`Ctrl+Shift+P`) for commands such as `Terminal: Split Right` and `Workspace: Change Workspace Folder`. Changing a workspace root affects future terminals; it does not change the directory of a running shell.

| Shortcut | Action |
| --- | --- |
| Ctrl+Alt+Right | Next tab |
| Ctrl+Alt+Left | Previous tab |
| Ctrl+Alt+Down | Next workspace |
| Ctrl+Alt+Up | Previous workspace |
| Ctrl+N | New workspace |
| Ctrl+Alt+R | Rename workspace |
| Ctrl+T | New tab |
| Ctrl+W | Close current tab and its panes |
| Ctrl+Shift+W | Close the focused pane |
| Ctrl+backtick | Focus the terminal pane |
| Ctrl+= | Increase terminal font size |
| Ctrl+- | Decrease terminal font size |
| Ctrl+Mouse Wheel | Change terminal font size |
| Ctrl+? | Keyboard shortcut help |
| Ctrl+, | Settings |
| Ctrl+B | Toggle sidebar |
| Ctrl+Shift+P | Command palette |
| Alt+Shift+D | Automatic split direction |
| Alt+Shift++ | Split pane right |
| Alt+Shift+- | Split pane down |
| Alt+ArrowUp | Move focus up |
| Alt+ArrowDown | Move focus down |
| Alt+ArrowLeft | Move focus left |
| Alt+ArrowRight | Move focus right |
| Alt+Shift+ArrowUp | Resize the nearest split up |
| Alt+Shift+ArrowDown | Resize the nearest split down |
| Alt+Shift+ArrowLeft | Resize the nearest split left |
| Alt+Shift+ArrowRight | Resize the nearest split right |
| Ctrl+C | Copy selected text, or interrupt the terminal |
| Ctrl+V | Paste clipboard text into the terminal |

Double-click a tab to rename it, drag tabs to reorder them, and right-click for tab actions. Drag splitters to resize panes. Closing the main window leaves terminal sessions running; closing a pane, tab, or workspace terminates its owned sessions.

## Runtime and persistence

The Go runtime owns ConPTY, terminal screen emulation, process monitoring, and persistence. The Wails bridge forwards application requests to the runtime. Closing the GUI detaches the interface without terminating terminal processes, so reopening the app can reconnect to the same sessions.

Runtime data is stored beside `paneacea-runtime.exe` as `paneacea.db`. Set `PANEACEA_DATA_DIR` before launching to use another data directory, for example:

```powershell
$env:PANEACEA_DATA_DIR = Join-Path $PWD '.data\dev'
.\build\bin\paneacea.exe
```

The Go runtime uses an independent protocol and database; existing Rust/WPF workspaces are not imported. A full runtime restart restores saved workspace, tab, pane, and launch configuration and starts fresh shell processes.

Closing the Paneacea window stops active registered agent panes while leaving ordinary terminal panes running. Starting Paneacea again relaunches those agent sessions from their captured session IDs and waits for the next prompt.

## Developer CLI

Build the application first, then use the CLI while the runtime is available:

```powershell
.\build\bin\paneacea-cli.exe workspace list
.\build\bin\paneacea-cli.exe workspace create Project C:\dev\project
.\build\bin\paneacea-cli.exe workspace switch Project
.\build\bin\paneacea-cli.exe tab create
.\build\bin\paneacea-cli.exe pane split --right
.\build\bin\paneacea-cli.exe agent list
```

The CLI also accepts a protocol method and JSON parameters directly. See [doc/protocol.md](doc/protocol.md) for the available methods and request fields.

```powershell
.\build\bin\paneacea-cli.exe tab.create '{"workspaceId":"ID","executable":"cmd.exe","arguments":[]}'
```

## Tests

Run the Go and frontend validation from the repository root:

```powershell
.\scripts\test-wails.ps1
```

For the browser interaction suite, run `npm run test:browser` from `frontend`. It covers terminal rendering, splitting, resizing, tab renaming, settings, and the command palette through a mocked Wails bridge.

## Architecture

```text
paneacea.exe (Wails + Svelte)
          |
  per-user named pipe
          |
paneacea-runtime.exe (Go)
      |             |
   ConPTY        SQLite
      |
Git Bash / pwsh / Windows PowerShell / cmd
```

See [Wails implementation, architecture, CLI, and validation](doc/wails-implementation.md), the [terminal rendering implementation](doc/paneacea-terminal-rendering.md), [the Wails plan](doc/panacea-plan-wails.md), and [the protocol reference](doc/protocol.md).
