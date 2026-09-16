# Engineering Guidelines

## Change scope

- Make the smallest change that solves the reported issue.
- Do not rename existing variables, methods, fields, or public types without a clear reason.
- Do not refactor or redesign unrelated code.
- Preserve existing behavior unless the change explicitly requires otherwise.
- Keep unrelated user changes intact.

## Code quality

- Follow the existing project style and patterns.
- Prefer clear, maintainable code over clever implementations.
- Handle errors explicitly and preserve useful diagnostics.
- Avoid unnecessary dependencies and generated files.
- Keep public interfaces compatible unless a breaking change is intentional and documented.

## Testing

- Add tests for new behavior and bug fixes when practical.
- Run relevant existing tests after every code change.
- Test both normal behavior and important failure paths.
- Do not change committed tests merely to make them pass; report failures separately.
- Do not ignore flaky or environment-dependent failures without documenting them.

## User input and interactions

- Preserve existing input behavior unless the change intentionally modifies it.
- Test modified and unmodified input paths when changing keyboard, mouse, clipboard, or focus handling.
- Do not consume an input event unless the application intentionally owns it.
- Avoid logging user-entered content, clipboard contents, secrets, or other sensitive data.
- Treat `Escape` as cancel or dismiss for every popup, dialog, subwindow, and context menu.
- Treat `Enter` as the primary confirm or submit action for every popup and dialog; focused buttons keep their native action, and textareas keep `Enter` for new lines.

## Documentation

- Update user-facing documentation when behavior, configuration, or commands change.
- Document public interfaces and non-obvious behavior when needed.
- Keep comments focused on decisions and behavior that are not obvious from the code.
- Ask before adding substantial inline comments to existing code.

## Build and validation

- Use the repository's existing build and validation commands where possible.
- Keep formatting and static-analysis checks clean.
- Do not commit build output, logs, databases, caches, or temporary files.
- Run `git diff --check` before handing off changes.

## Safety

- Work only within the project directory unless the user explicitly authorizes otherwise.
- Preserve unrelated changes in the working tree.
- Confirm exact targets before deleting, moving, or overwriting files.
- Prefer recoverable operations for disruptive file changes.

## Keyboard Shortcuts

- priority 1: Use common popular key combinations
- priority 2: Use VSCode key combinations
- priority 3: Ctrl+Alt+<key>
- priority 4: Ctrl+Alt+Shift+<key>
