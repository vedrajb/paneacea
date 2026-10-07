# Instance management handoff

Status: **parked — awaiting decision**. Branch context: `refactor/merge-to-1-app` (single-process merge, uncommitted at time of writing).

## Question

Can Paneacea support multiple app instances?

## Current behaviour (after the single-process merge)

- `paneacea.exe` hosts the runtime in-process (`internal/desktop/host.go`): it owns the ConPTY shells, `paneacea.db`, and the per-user named pipe used by `paneacea-cli` / `pca`.
- One instance per **install folder**. The pipe name and the Wails single-instance lock both come from `ipc.InstanceID()` = user SID + hash of the executable's folder (`internal/ipc/pipe_windows.go`).
- Launching again from the same folder focuses the existing window (`App.SecondInstance`).
- If the pipe is already taken (e.g. an old `paneacea-runtime.exe`), the host refuses to open the database, so two runtimes never write one DB.
- Copies installed in **different folders** already run side by side (different pipe, lock, and default data folder).
- `PANEACEA_DATA_DIR` changes where the DB lives but **not** the instance key, so two instances from one install with different data folders are currently blocked by the lock.

## Why "one per data set" is required

Each instance owns live shells, the DB, and the pipe. Two instances sharing one `paneacea.db` would overwrite each other's layout and terminal history.

## Options

| | Option | What the user gets | Effort | Trade-offs |
|---|---|---|---|---|
| **A** | **Independent instances, one per data folder** (recommended) | e.g. `work` and `personal` instances side by side, each with its own workspaces, shells, history, and CLI pipe | Small | CLI must target the matching data folder (same `PANEACEA_DATA_DIR` or a new `--data` flag); shells inside each instance inherit it automatically |
| **B1** | **Multiple windows sharing one workspace set — secondary windows as clients** | Several windows showing the same state | Large | Wails v2 supports one window per process, so extra windows are extra processes talking to the first over the pipe — essentially the two-process design just removed. Closing the first window kills every window's shells unless the runtime returns to a background process |
| **B2** | **Multiple windows sharing one workspace set — Wails v3 multi-window** | Several windows in one process | Large | Wails v3 is still alpha; framework migration of `cmd/paneacea`, `internal/desktop`, and frontend bindings |
| — | Independent instances sharing one DB | — | — | Not safe: concurrent writers corrupt layout/history. Rejected |

## Recommendation

Option **A** if the goal is separate contexts (work/personal, per-project). It keeps the single-process model and needs no framework change.

## Option A — implementation sketch

1. `internal/ipc/pipe_windows.go`: derive `InstanceID()` from the **data folder** (user SID + hash of the resolved, lower-cased data directory) instead of the executable folder. Keep the pipe prefix `paneacea-go-`.
2. Move data-folder resolution (`PANEACEA_DATA_DIR`, else executable folder) from `internal/desktop/host.go` into a shared place (e.g. `internal/ipc` or a small `internal/paths` package) so the GUI, host, and CLI resolve it identically.
3. Add `--data <dir>` to `paneacea.exe` / `paneacea-cli.exe` / `pca`: it sets `PANEACEA_DATA_DIR` for the process before anything resolves paths. `cmd/paneacea/main.go` currently treats any argument as CLI mode, so parse `--data` first and only then decide GUI vs CLI.
4. `ipc.Ensure`: when starting the app for the CLI, pass `--data <dir>` (or the environment variable) so the launched window opens the same data set.
5. Panes: set `PANEACEA_DATA_DIR` in each shell's environment (`internal/daemon/launch.go` / pane environment) so `pca` inside a pane talks to its own instance.
6. Window title: include the data-folder name when it is not the default, so instances are distinguishable.
7. Docs: README "Runtime and persistence" and `doc/wails-implementation.md`.

### Validation for option A

- Unit: `InstanceID()` differs for different data folders and is stable across case/trailing-slash variants of the same folder.
- Desktop: two hosts with different `PANEACEA_DATA_DIR` both start and serve separate pipes; two hosts with the same data folder → second refuses the DB.
- CLI: `paneacea-cli --data X workspace list` starts/targets instance X only; a shell inside instance X reaches X without flags.
- Manual: launch `work` and `personal` side by side; close one — the other's shells keep running.

### Risks for option A

- Existing installs: the instance key changes from executable folder to data folder. With the default data folder (the executable's folder) both hash the same path, so the pipe and lock names stay the same if the path is normalised identically; add a test for this. No data migration is needed either way — the DB path is unchanged.
- Users who relied on two installs sharing one `PANEACEA_DATA_DIR` would now get the "already running" guard instead of silent DB conflicts (intended).

## Open questions for the user

1. Which model is wanted: **A** (independent instances per data folder) or **B** (multiple windows sharing one workspace set)?
2. If A: is a `--data` flag wanted, or is `PANEACEA_DATA_DIR` enough?
3. Should the single-process merge on `refactor/merge-to-1-app` be committed before starting this work?
