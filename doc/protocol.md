# Paneacea protocol

Transport: local Windows named pipes, UTF-8 newline-delimited JSON, protocol version 1. Requests are limited to 1 MiB. Each request contains an opaque `id`, a `method`, and a `params` object. Method failures are returned without terminating the command connection.

```json
{"id":"1","method":"workspace.create","params":{"name":"Project","rootDirectory":"C:\\dev\\project"}}
```

```json
{"id":"1","ok":true,"result":{"workspaces":[],"panes":{},"activeWorkspaceId":null,"settings":{}}}
```

The result shown above is illustrative; workspace creation returns the updated full state. Errors have `ok:false` and an `error` string. State mutations are saved transactionally before success is returned. A failed save rolls back state and any newly launched terminal.

| Method | Required parameters | Result |
| --- | --- | --- |
| `state.get`, `workspace.list` | none | Full state |
| `workspace.create` | `name`, `rootDirectory` | Full state; workspace starts without tabs |
| `workspace.rename` | `workspaceId`, `name` | Full state |
| `workspace.setRoot` | `workspaceId`, `rootDirectory` | Full state |
| `workspace.switch` | `workspaceId` | Full state |
| `workspace.close` | `workspaceId` | Full state; terminates all owned shells |
| `tab.create` | `workspaceId` | Full state |
| `tab.rename` | `tabId`, `title` | Full state |
| `tab.focus` | `tabId` | Full state |
| `tab.close` | `tabId` | Full state; terminates owned shells |
| `pane.split` | `paneId`, `orientation` | Full state |
| `pane.focus` | `paneId` | Full state |
| `pane.close` | `paneId` | Full state; terminates shell |
| `pane.resize` | `tabId`, `path`, `ratio` | Full state |
| `pane.sendInput` | `paneId`, `data` | Empty object |
| `terminal.resize` | `paneId`, `rows`, `columns` | Empty object |
| `terminal.attach` | `paneId` | Initial screen, restoration metadata, and then output events |
| `terminal.viewport.set` | `paneId`, `offset` | Current scroll offset from the bottom |
| `settings.set` | `key`, `value` | Full state |

`tab.create` and `pane.split` accept optional `executable`, `arguments` (string array), and `environment` (string map). When `executable` is omitted, the runtime uses the persisted `settings.defaultShell` object. A new installation selects the first available profile in this order: Git Bash, PowerShell 7, Windows PowerShell, then Command Prompt. When selecting a different executable explicitly, provide its arguments, including `[]` if none. No command string is passed through an intermediate shell.

The MVP settings panel stores a default shell like this:

```json
{"key":"defaultShell","value":{"id":"git-bash","executable":"C:\\Program Files\\Git\\usr\\bin\\bash.exe","arguments":["--login","-i"]}}
```

The supported UI profiles are `git-bash`, `pwsh`, `powershell`, and `cmd`. Existing panes retain their persisted executable and arguments when the default changes.

`orientation` is `vertical` (left/right) or `horizontal` (top/bottom). A split `path` is an array of `0` (first child) and `1` (second child); `[]` selects the root. Ratios must be between `0.1` and `0.9`. Terminal dimensions are separate: 1–500 rows, 1–1000 columns.

An attach request uses a dedicated connection:

```json
{"id":"attach","ok":true,"result":{"data":"BASE64_VT_BYTES","exited":false,"viewportOffset":0,"restored":false}}
{"event":"pane.output","paneId":"...","data":"BASE64_VT_BYTES"}
{"event":"terminal.exited","paneId":"..."}
```

Base64 preserves arbitrary byte boundaries; clients need an incremental UTF-8 decoder. The snapshot and output subscription are established under one lock so output is neither lost nor duplicated across attachment. The runtime answers standard cursor/status/device-attribute queries itself, including when no client is attached. Closing this connection detaches; it does not kill the shell. Use another connection for input, resizing, and commands.

`viewportOffset` is the current number of scrollback rows above the bottom of the pane. It is not persisted with terminal history. `restored` is true when the snapshot includes history from a previous runtime generation. The client should apply the viewport after its scrollbar has been populated.

## Per-pane persistence

Pane layout, launch configuration, current working directory, terminal dimensions, and saved terminal history are associated with the pane ID inside its tab. Saved history is encrypted with Windows DPAPI for the current Windows user. A restored pane opens at the top of its terminal history; scroll position is not persisted. The `terminalHistoryLines` setting accepts `0`, `500`, `2000`, `5000`, `10000`, or `25000`; the default is `2000`, and `0` disables persisted history.

Interactive PowerShell and Git Bash sessions emit OSC 7 working-directory notifications through temporary runtime-provided prompt integration. The runtime validates local paths and falls back to the last saved directory when integration is unavailable.

When the runtime restarts, it restores the saved terminal snapshot and then launches a new shell in the saved directory. Shell variables, running jobs, and live full-screen application processes are not resumed. The restored terminal opens at the top of its history.

Example with PowerShell-generated JSON:

```powershell
$request = @{ workspaceId = 'WORKSPACE-ID'; executable = 'pwsh.exe'; arguments = @('-NoLogo') } | ConvertTo-Json -Compress
./target/debug/paneacea.exe tab.create $request
```

Workspace/tab/pane metadata events, explicit agent operations, and automatic client resynchronization from the full plan are not part of protocol version 1.
