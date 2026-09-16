import { describe, it, expect, vi } from "vitest";
import { handleKey, handleWheel, resolve, type Action } from "./actions";

function key(value: string, code: string, modifiers: string): KeyboardEvent {
  return {
    key: value,
    code,
    type: "keydown",
    repeat: false,
    isComposing: false,
    ctrlKey: modifiers.includes("Ctrl"),
    altKey: modifiers.includes("Alt"),
    shiftKey: modifiers.includes("Shift"),
    metaKey: false,
    preventDefault: vi.fn(),
  } as unknown as KeyboardEvent;
}

const bindings: [string, string, string, Action][] = [
  ["ArrowRight", "ArrowRight", "Ctrl+Alt", "Terminal.NextTab"],
  ["ArrowLeft", "ArrowLeft", "Ctrl+Alt", "Terminal.PreviousTab"],
  ["r", "KeyR", "Ctrl+Alt", "Workspace.Rename"],
  ["`", "Backquote", "Ctrl", "Terminal.Focus"],
  ["t", "KeyT", "Ctrl", "Terminal.NewTab"],
  ["w", "KeyW", "Ctrl", "Terminal.CloseTab"],
  ["n", "KeyN", "Ctrl", "Workspace.New"],
  ["=", "Equal", "Ctrl", "Terminal.IncreaseFontSize"],
  ["-", "Minus", "Ctrl", "Terminal.DecreaseFontSize"],
  ["-", "NumpadSubtract", "Ctrl", "Terminal.DecreaseFontSize"],
  ["?", "Slash", "Ctrl+Shift", "Help.ShowShortcuts"],
  ["W", "KeyW", "Ctrl+Shift", "Terminal.ClosePane"],
  ["P", "KeyP", "Ctrl+Shift", "Paneacea.CommandPalette"],
  [",", "Comma", "Ctrl", "Paneacea.Settings"],
  ["D", "KeyD", "Alt+Shift", "Terminal.SplitPaneAuto"],
  ["_", "Minus", "Alt+Shift", "Terminal.SplitPaneDown"],
  ["+", "Equal", "Alt+Shift", "Terminal.SplitPaneRight"],
  ...(["Up", "Down", "Left", "Right"] as const).flatMap((direction) => [
    [
      `Arrow${direction}`,
      `Arrow${direction}`,
      "Alt",
      `Terminal.MoveFocus${direction}`,
    ] as [string, string, string, Action],
    [
      `Arrow${direction}`,
      `Arrow${direction}`,
      "Alt+Shift",
      `Terminal.ResizePane${direction}`,
    ] as [string, string, string, Action],
  ]),
];

describe("Rust/C# shortcut parity", () => {
  it.each(bindings)(
    "%s (%s), %s executes %s once without terminal input",
    (value, code, modifiers, action) => {
      const event = key(value, code, modifiers),
        execute = vi.fn();
      const terminal = { hasSelection: () => false, getSelection: () => "" };
      expect(resolve(event)).toBe(action);
      expect(handleKey(event, terminal, execute, vi.fn(), vi.fn(), {})).toBe(
        false,
      );
      expect(event.preventDefault).toHaveBeenCalledOnce();
      expect(execute).toHaveBeenCalledExactlyOnceWith(action);
      for (const extra of [
        { type: "keyup" },
        { type: "keypress" },
        { repeat: true },
      ]) {
        handleKey(
          { ...event, ...extra } as KeyboardEvent,
          terminal,
          execute,
          vi.fn(),
          vi.fn(),
          {},
        );
      }
      expect(execute).toHaveBeenCalledOnce();
    },
  );
  it("can explicitly return Ctrl+W to the terminal", () => {
    expect(resolve(key("w", "KeyW", "Ctrl"), { "Ctrl+w": "" })).toBeNull();
  });
  it.each(["Ctrl", "Ctrl+Shift"])(
    "preserves copy/paste and control sequences with %s",
    (modifiers) => {
      for (const value of ["c", "v"])
        expect(
          resolve(key(value, `Key${value.toUpperCase()}`, modifiers)),
        ).toBeNull();
    },
  );
  it.each(bindings)(
    "ignores %s with extra Meta or composition",
    (value, code, modifiers) => {
      expect(
        resolve({ ...key(value, code, modifiers), metaKey: true }),
      ).toBeNull();
      expect(
        resolve({ ...key(value, code, modifiers), isComposing: true }),
      ).toBeNull();
    },
  );
});

describe("Ctrl+mouse wheel font sizing", () => {
  it.each([-120, 120])("handles wheel delta %s once", (deltaY) => {
    const execute = vi.fn(),
      event = {
        ctrlKey: true,
        deltaY,
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
      } as unknown as WheelEvent;
    handleWheel(event, execute);
    expect(execute).toHaveBeenCalledExactlyOnceWith(
      deltaY < 0 ? "Terminal.IncreaseFontSize" : "Terminal.DecreaseFontSize",
    );
    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(event.stopPropagation).toHaveBeenCalledOnce();
  });
  it.each([
    { ctrlKey: false, deltaY: 120 },
    { ctrlKey: true, deltaY: 0 },
  ])("preserves scrolling for %j", (extra) => {
    const execute = vi.fn(),
      event = {
        ...extra,
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
      } as unknown as WheelEvent;
    handleWheel(event, execute);
    expect(execute).not.toHaveBeenCalled();
    expect(event.preventDefault).not.toHaveBeenCalled();
  });
});
