# Paneacea Per-Pane State Persistence Plan

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
- Keep `terminal.viewport.set` for live scrolling without persisting its value.
- Extend `terminal.attach` with restoration metadata while continuing to use the existing terminal snapshot data field.

## Restore behavior

On runtime startup:

1. Load and decrypt each pane's saved history.
2. Recreate the terminal parser using the saved dimensions and history.
3. Launch a new shell in the saved working directory.
4. Return the restored snapshot when the GUI attaches.

The GUI opens restored panes at the top of their saved terminal history. Scroll position is a live terminal concern and is not restored after a runtime restart.

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
- Runtime restart restoring history and launching a new shell at the top of the restored history
- GUI attach preserving the current live viewport independently for every pane

Update the README and protocol documentation to describe the persistence guarantees and the fact that shell processes themselves are relaunched.
