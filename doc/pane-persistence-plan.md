# Paneacea Per-Pane State Persistence Plan

## Investigation status — 2026-09-24

Terminal output history is not currently persisted by the Go runtime. The shutdown fix preserves pane records and the layout, but does not save terminal output.

Evidence from the current implementation:

- `internal/persistence/store.go` creates only `application_state` and saves the JSON model. Read-only inspection of the portable database confirmed that this is its only table.
- `internal/model/model.go` stores pane configuration, directories, dimensions, and process metadata, but no terminal screen or history.
- `internal/daemon/output.go` keeps a 2 MiB output ring and a terminal emulator in memory. Its attach snapshots are generated from that live emulator; they are not written to SQLite.
- `internal/daemon/runtime.go` creates a new output buffer and emulator in `launch`. It currently caps emulator scrollback at 2,000 lines, regardless of a larger frontend scrollback setting. `Close` does not save terminal output.
- `frontend/src/components/XtermView.svelte` creates a fresh xterm instance, resets it on a snapshot, and displays the snapshot supplied by the runtime. After a runtime restart, that snapshot contains only the new shell's output.
- `doc/protocol.md` describes saved history, DPAPI, `terminalHistoryLines`, restoration metadata, and `terminal.viewport.set`, but those features are not implemented in the current Go code.

This plan covers visible terminal output and scrollback. Git Bash command recall is saved separately in a plaintext file for each pane under the runtime data directory's `bash-history` folder. The prompt hook flushes commands as they complete, so a forced runtime exit does not depend on Bash's normal exit to save them.

Implementation is in place: the history table, DPAPI protection, runtime checkpoints and restoration, saved-history setting, attach viewport metadata, tests, and documentation are included. Windows shutdown handling preserves pane records. A physical reboot check with disposable data remains manual validation.

Output from a previous runtime cannot be recovered from the existing application-state database. Implementing this plan will preserve output captured from that point onward. No restoration or replacement of the current database contents is part of this planning task.

## Goal

Persist the state of every terminal pane independently within its tab. Restoring a workspace must preserve the tab layout and each pane's terminal context, while starting a new shell process when the runtime has been restarted.

## State to persist

Each pane is already associated with a tab through its pane record. Extend that persistence to include:

- Current working directory
- Terminal history and visible screen
- Terminal dimensions
- Existing pane launch information, runtime identifiers, and generation metadata

The existing tab state remains responsible for the pane tree, split orientation and ratios, tab order, and active pane.

Live shell processes, shell variables, running jobs, and full-screen application processes cannot be resumed after the runtime exits. They are relaunched from the saved pane configuration.

## Runtime and storage design

- Add a dedicated SQLite terminal-history table keyed by pane ID; do not include history in the existing full-state rewrite.
- Encrypt each saved history blob with Windows DPAPI for the current Windows user.
- Save dirty history periodically and flush it during clean runtime shutdown. A crash may lose only the most recent flush interval.
- Ignore corrupt or undecryptable history and launch the pane normally, without preventing runtime startup.
- Remove saved history when its pane, tab, or workspace is closed.
- Implement `terminal.viewport.set` for live scrolling without persisting its value; the documented endpoint is currently absent.
- Extend `terminal.attach` with restoration metadata while continuing to use the existing terminal snapshot data field.

## Restore behavior

On runtime startup:

1. Load and decrypt each pane's saved history.
2. Recreate the terminal parser using the saved dimensions and history.
3. Launch a new shell in the saved working directory.
4. Return the restored snapshot when the GUI attaches.

The GUI opens restored panes at the bottom of their saved terminal history. Scroll position is a live terminal concern and is not restored after a runtime restart.

## Working-directory tracking

Use temporary shell integration for interactive PowerShell and Git Bash sessions:

- Emit OSC 7 working-directory notifications from the prompt.
- Parse fragmented OSC 7 sequences in the runtime.
- Preserve the existing PowerShell prompt and Git Bash `PROMPT_COMMAND`.
- Do not modify user profile files.
- Fall back to the last valid saved directory for unsupported shells or invalid notifications.

## Saved-history setting

Add a terminal setting with these choices:

```text
Off, 500, 2,000, 5,000, 10,000, 25,000 lines
```

The default is 2,000 lines. Selecting Off deletes persisted terminal history; in-memory terminal behavior remains available for the current runtime session.

## Validation

Add coverage for:

- Multiple panes in one tab restoring independent directories, history, and dimensions
- History limits and the Off setting
- DPAPI round trips and corrupt or wrong-user data
- Fragmented OSC 7 parsing and path validation
- History surviving ordinary state saves and being deleted with panes, tabs, or workspaces
- Runtime restart restoring history and launching a new shell at the bottom of the restored history
- GUI attach preserving the current live viewport independently for every pane

Update the README and protocol documentation to describe the persistence guarantees and the fact that shell processes themselves are relaunched.

## Implementation sequence

### 1. Add history storage and settings

Files: `internal/persistence/store.go`, `internal/model/model.go`, and new persistence helpers.

- Add an additive SQLite migration for `terminal_history`, keyed by pane ID, with a payload format version, dimensions, save timestamp, and encrypted snapshot blob. Preserve all existing workspace and pane records.
- Add history load/save/delete operations separately from application-state serialization. Encrypt and decrypt through Windows DPAPI, as specified above.
- Add `terminalHistoryLines` with a 2,000-line default for existing settings that omit the field. Distinguish an absent value from an explicit zero so Off remains Off after restart.
- Keep the saved-history setting distinct from live scrollback. Define a byte limit as well as a line limit to bound storage and memory use.

### 2. Define a restorable terminal snapshot

Files: `internal/daemon/output.go` and new snapshot helpers as needed.

- Serialize complete rendered scrollback rows and the visible screen, preserving text, colors, Unicode, wrapping information, and dimensions where supported by the emulator.
- Do not persist the arbitrary trailing bytes of the output ring: they can begin inside an escape sequence and cannot reconstruct prior screen state reliably.
- Treat persisted output as historical content. A fresh shell must start with usable terminal modes; replay must not send old terminal-query responses or input to the new shell. Cover alternate-screen applications explicitly.
- Track dirty generations under the output lock. Copy a consistent snapshot under that lock, then encrypt and write outside the lock so terminal output is not blocked by SQLite I/O.
- Account for the existing 512 KiB attach-history cap and 4 MiB IPC message limit, including base64 expansion. Use bounded restore chunks if the selected history exceeds one response; do not silently promise 25,000 lines while delivering only the current capped snapshot.

### 3. Save history throughout the runtime lifecycle

Files: `internal/daemon/runtime.go`, `internal/daemon/output.go`, and persistence helpers.

- Checkpoint dirty panes every two seconds. Mark a generation saved only after the corresponding write succeeds, leaving newer output dirty.
- On clean runtime shutdown, stop new lifecycle mutations and flush output before disposing of sessions and buffers. Coordinate the final flush with queued output callbacks.
- On Windows shutdown, preserve the pane and its last saved history; attempt a final checkpoint where possible. Periodic checkpoints remain necessary because Windows termination or power loss can prevent a final flush.
- Delete history when a pane is deliberately closed or a normal shell exit removes its pane. Apply equivalent cleanup to tab/workspace removal. Serialize pruning with pending history writes so a late checkpoint cannot recreate deleted history.
- Off deletes saved history and disables subsequent checkpoints. Reducing the limit trims saved history. Surface save failures without destroying the previous valid snapshot.

### 4. Restore before launching the shell

Files: `internal/daemon/runtime.go`, `internal/daemon/output.go`.

- Load each pane's history using its stable pane ID, validate the format and size, decrypt it, and hydrate the terminal state before consuming fresh shell output.
- Restore the saved dimensions, then launch a new shell in the saved working directory. Append new output after historical output and reset stream sequence bookkeeping for the new runtime.
- Ensure shell startup clear-screen sequences cannot unintentionally erase the restored scrollback.
- Missing, corrupt, unsupported, or undecryptable snapshots should affect only that pane's history. Continue launching the pane and report the history restore failure.

### 5. Add frontend and protocol support

Files: `internal/ipc/protocol.go`, `internal/desktop/app.go`, `frontend/src/services/backend.ts`, `frontend/src/components/XtermView.svelte`, `frontend/src/App.svelte`.

- Add restoration metadata and set the IPC message limit to accommodate a 2 MiB history snapshot encoded as base64. Keep snapshot delivery and subsequent output sequences consistent to prevent duplicate or missing output.
- Add the saved-history choices from this plan to Settings. Ensure the effective frontend scrollback capacity can display the selected restored history.
- Populate history before applying the restored viewport. Open a restored pane at the bottom once; ordinary output updates must not repeatedly jump the user's scroll position.
- Preserve per-pane viewport positions across GUI detach/reattach within the same runtime. Start with the specified bottom-of-history behavior after a runtime restart.
- Update `doc/protocol.md` to match the implemented request/response flow and label any remaining planned behavior clearly.

### 6. Validate and document the guarantees

- Add focused persistence, output, runtime, and frontend tests for the scenarios listed above. Include three panes with different output markers, colored/Unicode content, working directories, and dimensions across a real SQLite close/reopen.
- Exercise simulated Windows shutdown, ordinary runtime restart, GUI-only reattachment, explicit pane removal, failed writes, Off, and corrupt encrypted data.
- Verify history near and above the current attach limit, output arriving during checkpoints, and startup output arriving while the client restores history.
- After automated checks, perform a user-controlled reboot check using disposable panes and an isolated database.
- Document that existing unsaved history cannot be reconstructed, live processes are relaunched, and abrupt termination may lose output since the last successful checkpoint. A two-second timer is a target interval, not an unconditional durability guarantee.

## Completion criteria

After creating output in multiple panes and restarting the runtime, every pane retains its own saved output and layout, starts a fresh usable shell, and displays subsequent output without duplication. Windows shutdown must retain both the pane record and its last successful history checkpoint. Closing panes and selecting Off must remove their persisted history. Existing application-state data must survive the additive migration.
