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
| `terminal.viewport.set` | `paneId`, `offset` | Saves the pane's live scroll offset until it closes or the runtime exits |
| `settings.set` | `key`, `value` | Full state |

The desktop lifecycle uses two additional runtime methods. `agent.stop` stops active registered agent panes and marks them for restoration while leaving ordinary terminal panes running. `agent.restore` relaunches panes marked by `agent.stop`; the resumed agent is restored without sending a new prompt. These calls are intended for the GUI lifecycle and take no parameters.

`tab.create` and `pane.split` accept optional `executable`, `arguments` (string array), and `environment` (string map). When `executable` is omitted, the runtime uses the persisted `settings.defaultShell` object. A new installation selects the first available profile in this order: Git Bash, PowerShell 7, Windows PowerShell, then Command Prompt. When selecting a different executable explicitly, provide its arguments, including `[]` if none. No command string is passed through an intermediate shell.

The MVP settings panel stores a default shell like this:

```json
{"key":"defaultShell","value":{"id":"git-bash","executable":"C:\\Program Files\\Git\\usr\\bin\\bash.exe","arguments":["--login","-i"]}}
```

The supported UI profiles are `git-bash`, `pwsh`, `powershell`, and `cmd`. Existing panes retain their persisted executable and arguments when the default changes.

`orientation` is `vertical` (left/right) or `horizontal` (top/bottom). A split `path` is an array of `0` (first child) and `1` (second child); `[]` selects the root. Ratios must be between `0.1` and `0.9`. Terminal dimensions are separate: 1–500 rows, 1–1000 columns.

An attach request uses a dedicated connection:

```json
{"id":"attach","ok":true,"result":{"data":"BASE64_VT_BYTES","exited":false,"viewportOffset":0,"restored":true}}
{"event":"pane.output","paneId":"...","data":"BASE64_VT_BYTES"}
{"event":"terminal.exited","paneId":"..."}
```

Base64 preserves arbitrary byte boundaries; clients need an incremental UTF-8 decoder. The snapshot and output subscription are established under one lock so output is neither lost nor duplicated across attachment. The runtime answers standard cursor/status/device-attribute queries itself, including when no client is attached. Closing this connection detaches; it does not kill the shell. Use another connection for input, resizing, and commands.

`viewportOffset` is the current number of scrollback rows above the bottom of the pane. `terminal.viewport.set` stores it in runtime memory so a detached pane can return to the same view; it resets when the runtime restarts. `restored` is true on the first attach when the runtime loaded saved history. The client should apply the viewport after writing the snapshot and populating the scrollbar.

## Per-pane persistence

Pane layout, launch configuration, current working directory, terminal dimensions, and saved terminal history are associated with the pane ID inside its tab. Saved history is encrypted with Windows DPAPI for the current Windows user in a separate SQLite table. History is checkpointed every two seconds and flushed after shells stop on clean runtime shutdown. Each pane is limited to 2 MiB of encoded terminal snapshot data; history that exceeds this cap retains the previous successful checkpoint. `terminalHistoryLines` accepts `0`, `500`, `2000`, `5000`, `10000`, or `25000`; the default for new and existing states is `2000`, and `0` disables and deletes persisted history while keeping in-memory scrollback. The byte cap can truncate saved lines before the selected line count. Existing output from before history persistence was installed is not recoverable.

Interactive PowerShell and Git Bash sessions emit OSC 7 working-directory notifications through temporary runtime-provided prompt integration. The runtime validates local paths and falls back to the last saved directory when integration is unavailable.

When the runtime restarts, it restores the saved terminal snapshot and then launches a new shell in the saved directory. Shell variables, running jobs, and live full-screen application processes are not resumed. Alternate-screen application content is not restored; the primary scrollback is retained. The first attach after runtime restart opens at the top of saved history. A clean shutdown flushes history, while abrupt termination may lose output since the last successful checkpoint.

Example with PowerShell-generated JSON:

```powershell
$request = @{ workspaceId = 'WORKSPACE-ID'; executable = 'pwsh.exe'; arguments = @('-NoLogo') } | ConvertTo-Json -Compress
./target/debug/paneacea.exe tab.create $request
```

Workspace/tab/pane metadata events, explicit agent operations, and automatic client resynchronization from the full plan are not part of protocol version 1.
