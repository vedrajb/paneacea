# Paneacea

A Windows terminal workspace app built with Wails, Svelte, and a persistent Go runtime. The earlier Rust/C# source is archived under `archive/rust-csharp`.

## Requirements

- Windows 10 version 1809 or later, or Windows 11 (x64)
- WebView2 Runtime
- Microsoft Edge (used to render the Windows application icon during builds)
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

This recreates `paneacea-portable` and writes `paneacea-portable.zip`.

The PowerShell build script can also be run directly. Omit `-Run` to build without launching:

```powershell
.\scripts\build-wails.ps1 -Configuration Release
.\scripts\build-wails.ps1 -Configuration Release -Run
```

The build writes intermediate executables to `build\bin` and creates the portable package at `paneacea-portable`:

```text
paneacea-portable/
├── paneacea.exe
├── paneacea-cli.exe
├── conpty.dll
├── OpenConsole.exe
├── conpty-LICENSE.txt
├── Logs/
├── config.toml
├── pca.cmd
└── pca
```

`config.toml` is seeded with default user settings and is preserved when the package is rebuilt.

Add the package folder to your `PATH` to launch Paneacea as `pca` from cmd, PowerShell, or Git Bash (`pca.cmd` serves cmd and PowerShell; the extensionless `pca` serves Git Bash and other MSYS shells):

```text
pca                      Open Paneacea, or focus it if already open; returns immediately
pca workspace list       Run a CLI command and print the result (starts Paneacea if needed)
```

The build regenerates the executable and taskbar icon from `icons\paneacea-app-icon.svg`. Use `build-wails.ps1` when building the desktop application; a direct `go build` uses the most recently generated Windows icon resource.

The intermediate build output contains:

- `paneacea.exe` — Wails desktop application with the built-in Go runtime
- `paneacea-cli.exe` — developer protocol CLI

Launch the intermediate build directly with:

```powershell
.\build\bin\paneacea.exe
```

Keep all three executables in the same directory. A separately installed Wails CLI is not required. The build script uses the workspace-local Go toolchain when available and otherwise uses Go from `PATH`.

## Usage

After launch, create a workspace, choose its root folder, and open a terminal tab. The first launch detects Git Bash, PowerShell 7, Windows PowerShell, and Command Prompt. The collapsed left rail opens the workspace selector, creates workspaces, and provides Settings. Use Settings to choose the default shell, font size, in-memory scrollback, saved terminal history, appearance, and custom keybindings. Reuse `frontend/src/theme.css` as the color-token template for additional themes.

Press `Ctrl+B` to open the centered workspace selector. Use Up/Down to choose a workspace, Enter to switch, or Escape to cancel.

Use the command palette (`Ctrl+Shift+P`) for commands such as `Terminal: Split Right` and `Workspace: Change Workspace Folder`. Changing a workspace root affects future terminals; it does not change the directory of a running shell.

| Shortcut | Action |
| --- | --- |
| Ctrl+Alt+Right | Next tab |
| Ctrl+Alt+Left | Previous tab |
| Ctrl+B | Open workspace selector |
| Ctrl+N | New workspace |
| Ctrl+T | New tab |
| Ctrl+W | Close current tab and its panes |
| Ctrl+Shift+W | Close the focused pane |
| Ctrl+backtick | Focus the terminal pane |
| Ctrl+= | Increase terminal font size |
| Ctrl+- | Decrease terminal font size |
| Ctrl+Mouse Wheel | Change terminal font size |
| Ctrl+? | Keyboard shortcut help |
| Ctrl+, | Settings |
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

Double-click a tab to rename it, drag tabs to reorder them, and right-click for tab actions. Drag splitters to resize panes. Closing the main window ends all terminal sessions after saving the layout; closing a pane, tab, or workspace terminates its owned sessions.

## Runtime and persistence

`paneacea.exe` is a single process: its Go runtime owns ConPTY, terminal screen emulation, process monitoring, and persistence, and the Wails bridge calls it directly. The same process serves a per-user named pipe for `paneacea-cli.exe` and `pca`. Only one Paneacea runs per install folder; launching it again focuses the open window. Closing the window saves the layout and terminal history, then ends the shells, so nothing keeps running in the background. Reopening Paneacea relaunches the saved panes.

The build bundles Microsoft's ConPTY (`conpty.dll` and `OpenConsole.exe` from the pinned `Microsoft.Windows.Console.ConPTY` NuGet package, fetched by `scripts/fetch-conpty.ps1`) beside `paneacea.exe`. Unlike the inbox Windows ConPTY it passes application modes such as mouse tracking through to the terminal. Without those files the runtime falls back to the inbox ConPTY; set `PANEACEA_CONPTY=inbox` to force the fallback, or to an absolute `conpty.dll` path to test another build.

Runtime data is stored beside `paneacea.exe` as `paneacea.db`; logs go to `Logs\paneacea.log`. Set `PANEACEA_DATA_DIR` before launching to use another data directory, for example:

```powershell
$env:PANEACEA_DATA_DIR = Join-Path $PWD '.data\dev'
.\build\bin\paneacea.exe
```

The Go runtime uses an independent protocol and database; existing Rust/WPF workspaces are not imported. A normal shell exit closes its pane and removes its saved record, so only active shell panes are restored after a runtime restart. Shell exits during Windows shutdown or reboot preserve their panes and saved layout. Restored panes use their saved workspace, tab, pane, and launch configuration to start fresh shell processes. Terminal output history is encrypted for the current Windows user and checkpointed every two seconds; restored panes start a fresh shell rather than replaying saved output. Settings controls the saved line limit; Off deletes saved history. Git Bash panes save separate command recall files in the data directory's `bash-history` folder. These Bash files are plaintext, unlike encrypted terminal output snapshots. Abrupt shutdown may lose output since the last successful checkpoint, and output from before this feature was added cannot be recovered.

Closing the Paneacea window stops active registered agent panes and ends ordinary terminal panes. Starting Paneacea again relaunches the panes, and agent sessions resume from their captured session IDs.

## Developer CLI

Build the application first. The CLI talks to the running app and starts Paneacea in the background if it is not open, so the calling terminal is never blocked:

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
paneacea.exe (Wails + Svelte + Go runtime) <-- per-user named pipe --- paneacea-cli.exe / pca
      |             |
   ConPTY        SQLite
      |
Git Bash / pwsh / Windows PowerShell / cmd
```

See [Wails implementation, architecture, CLI, and validation](doc/wails-implementation.md), the [terminal rendering implementation](doc/paneacea-terminal-rendering.md), [the Wails plan](doc/panacea-plan-wails.md), and [the protocol reference](doc/protocol.md).
