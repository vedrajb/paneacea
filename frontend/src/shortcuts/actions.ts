export const actions = {
  "Terminal.NewTab": "Terminal: New Tab",
  "Terminal.CloseTab": "Terminal: Close Tab",
  "Terminal.Focus": "Terminal: Focus Terminal",
  "Terminal.IncreaseFontSize": "Terminal: Increase Font Size",
  "Terminal.DecreaseFontSize": "Terminal: Decrease Font Size",
  "Terminal.ClosePane": "Terminal: Close Pane",
  "Terminal.SplitPaneAuto": "Terminal: Split Pane",
  "Terminal.SplitPaneRight": "Terminal: Split Right",
  "Terminal.SplitPaneDown": "Terminal: Split Down",
  "Terminal.MoveFocusUp": "Terminal: Focus Up",
  "Terminal.MoveFocusDown": "Terminal: Focus Down",
  "Terminal.MoveFocusLeft": "Terminal: Focus Left",
  "Terminal.MoveFocusRight": "Terminal: Focus Right",
  "Terminal.ResizePaneUp": "Terminal: Resize Up",
  "Terminal.ResizePaneDown": "Terminal: Resize Down",
  "Terminal.ResizePaneLeft": "Terminal: Resize Left",
  "Terminal.ResizePaneRight": "Terminal: Resize Right",
  "Terminal.OpenTabRenamer": "Terminal: Rename Tab",
  "Terminal.Restart": "Terminal: Restart Exited Pane",
  "Terminal.NextTab": "Terminal: Next Tab",
  "Terminal.PreviousTab": "Terminal: Previous Tab",
  "Workspace.New": "Workspace: New Workspace",
  "Workspace.Close": "Workspace: Close Workspace",
  "Workspace.Next": "Workspace: Next",
  "Workspace.Previous": "Workspace: Previous",
  "Workspace.Switch": "Workspace: Switch Workspace",
  "Workspace.Rename": "Workspace: Rename",
  "Workspace.ChangeRoot": "Workspace: Open Folder",
  "Paneacea.CommandPalette": "Paneacea: Command Palette",
  "Paneacea.Settings": "Preferences: Open Settings",
  "Paneacea.Reconnect": "Paneacea: Reconnect Runtime",
  "Agent.Resume": "Agent: Resume Captured Session",
  "Help.ShowShortcuts": "Help: Keyboard Shortcuts",
  "Paneacea.ToggleSidebar": "View: Toggle Side Bar Visibility",
} as const;
export type Action = keyof typeof actions;
export const defaults: Record<string, Action> = {
  "Ctrl+t": "Terminal.NewTab",
  "Ctrl+w": "Terminal.CloseTab",
  "Ctrl+n": "Workspace.New",
  "Ctrl+`": "Terminal.Focus",
  "Ctrl+=": "Terminal.IncreaseFontSize",
  "Ctrl+-": "Terminal.DecreaseFontSize",
  "Ctrl+Shift+/": "Help.ShowShortcuts",
  "Ctrl+Alt+ArrowDown": "Workspace.Next",
  "Ctrl+Alt+ArrowUp": "Workspace.Previous",
  "Ctrl+Alt+r": "Workspace.Rename",
  "Ctrl+Shift+w": "Terminal.ClosePane",
  "Alt+Shift+d": "Terminal.SplitPaneAuto",
  "Alt+Shift+-": "Terminal.SplitPaneDown",
  "Alt+Shift+=": "Terminal.SplitPaneRight",
  "Alt+ArrowUp": "Terminal.MoveFocusUp",
  "Alt+ArrowDown": "Terminal.MoveFocusDown",
  "Alt+ArrowLeft": "Terminal.MoveFocusLeft",
  "Alt+ArrowRight": "Terminal.MoveFocusRight",
  "Alt+Shift+ArrowUp": "Terminal.ResizePaneUp",
  "Alt+Shift+ArrowDown": "Terminal.ResizePaneDown",
  "Alt+Shift+ArrowLeft": "Terminal.ResizePaneLeft",
  "Alt+Shift+ArrowRight": "Terminal.ResizePaneRight",
  "Ctrl+Shift+p": "Paneacea.CommandPalette",
  "Ctrl+b": "Paneacea.ToggleSidebar",
  "Ctrl+,": "Paneacea.Settings",
  "Ctrl+Alt+ArrowRight": "Terminal.NextTab",
  "Ctrl+Alt+ArrowLeft": "Terminal.PreviousTab",
};
export function normalize(event: KeyboardEvent): string {
  let key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  if (
    event.getModifierState?.("AltGraph") &&
    /^Key[A-Z]$/.test(event.code)
  )
    key = event.code.slice(3).toLowerCase();
  if (event.code === "Equal" && event.shiftKey) key = "=";
  if (event.code === "Minus" && event.shiftKey) key = "-";
  if (event.code === "NumpadAdd") key = "+";
  if (event.code === "NumpadSubtract") key = "-";
  if (event.code === "Backquote" && !event.shiftKey) key = "`";
  if (event.shiftKey && (event.code === "Slash" || key === "?")) key = "/";
  return [
    event.ctrlKey && "Ctrl",
    event.altKey && "Alt",
    event.shiftKey && "Shift",
    event.metaKey && "Meta",
    key,
  ]
    .filter(Boolean)
    .join("+");
}

export function handleWheel(
  event: WheelEvent,
  execute: (action: Action) => void,
): void {
  if (!event.ctrlKey || event.deltaY === 0) return;
  event.preventDefault();
  event.stopPropagation();
  execute(
    event.deltaY < 0
      ? "Terminal.IncreaseFontSize"
      : "Terminal.DecreaseFontSize",
  );
}
export function resolve(
  event: KeyboardEvent,
  overrides: Record<string, string> = {},
): Action | null {
  if (event.isComposing) return null;
  const action = { ...defaults, ...overrides }[normalize(event)];
  if (event.getModifierState?.("AltGraph") && !action) return null;
  return action && action in actions ? (action as Action) : null;
}
export function shortcutsForAction(
  action: Action,
  overrides: Record<string, string> = {},
): string[] {
  return Object.entries({ ...defaults, ...overrides })
    .filter(([, boundAction]) => boundAction === action)
    .map(([shortcut]) => shortcut);
}
export function handleKey(
  event: KeyboardEvent,
  terminal: { hasSelection(): boolean; getSelection(): string },
  execute: (action: Action) => void,
  copy: (text: string) => Promise<void>,
  error: (message: string) => void,
  overrides: Record<string, string>,
): boolean {
  if (
    !event.isComposing &&
    event.ctrlKey &&
    !event.altKey &&
    !event.shiftKey &&
    !event.metaKey &&
    event.key.toLowerCase() === "c" &&
    terminal.hasSelection()
  ) {
    event.preventDefault();
    if (event.type === "keydown" && !event.repeat) {
      const text = terminal.getSelection();
      void Promise.resolve()
        .then(() => copy(text))
        .catch(() => error("Could not copy terminal selection."));
    }
    return false;
  }
  if (
    !event.isComposing &&
    event.ctrlKey &&
    !event.altKey &&
    !event.shiftKey &&
    !event.metaKey &&
    event.key.toLowerCase() === "v"
  )
    return false;
  const action = resolve(event, overrides);
  if (!action) return true;
  event.preventDefault();
  if (event.type === "keydown" && !event.repeat) execute(action);
  return false;
}
