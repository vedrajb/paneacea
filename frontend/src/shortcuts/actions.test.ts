import { describe, it, expect, vi } from "vitest";
import { resolve, handleKey, shortcutsForAction } from "./actions";
function key(
  key: string,
  overrides: Partial<KeyboardEvent> = {},
): KeyboardEvent {
  return {
    key,
    type: "keydown",
    code: "",
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    metaKey: false,
    isComposing: false,
    repeat: false,
    preventDefault: vi.fn(),
    ...overrides,
  } as unknown as KeyboardEvent;
}
describe("terminal key ownership", () => {
  it.each(["c", "z", "r", "v", " "])("passes Ctrl+%s to xterm", (value) => {
    expect(resolve(key(value, { ctrlKey: true }))).toBeNull();
  });
  it.each(["Tab", "F1", "F12", "Escape", "Home", "End", "ArrowUp"])(
    "passes %s to xterm",
    (value) => {
      expect(resolve(key(value))).toBeNull();
    },
  );
  it("normalizes shifted split keys", () => {
    expect(
      resolve(key("+", { code: "Equal", shiftKey: true, altKey: true })),
    ).toBe("Terminal.SplitPaneRight");
    expect(
      resolve(key("_", { code: "Minus", shiftKey: true, altKey: true })),
    ).toBe("Terminal.SplitPaneDown");
  });
  it("allows disabling and remapping defaults", () => {
    const event = key("T", { ctrlKey: true, shiftKey: true });
    expect(resolve(event, { "Ctrl+Shift+t": "" })).toBeNull();
    expect(resolve(event, { "Ctrl+Shift+t": "Workspace.New" })).toBe(
      "Workspace.New",
    );
  });
  it("does not consume composition or extra modifiers", () => {
    expect(
      resolve(key("t", { ctrlKey: true, shiftKey: true, isComposing: true })),
    ).toBeNull();
    expect(
      resolve(key("t", { ctrlKey: true, shiftKey: true, metaKey: true })),
    ).toBeNull();
  });
  it("copies selection without interrupting, including failure", async () => {
    const copy = vi.fn().mockRejectedValue(new Error("denied")),
      error = vi.fn(),
      execute = vi.fn();
    expect(
      handleKey(
        key("c", { ctrlKey: true }),
        { hasSelection: () => true, getSelection: () => "text" },
        execute,
        copy,
        error,
        {},
      ),
    ).toBe(false);
    await vi.waitFor(() => expect(error).toHaveBeenCalled());
    expect(copy).toHaveBeenCalledWith("text");
    expect(execute).not.toHaveBeenCalled();
  });
  it("passes unselected Ctrl+C through", () => {
    expect(
      handleKey(
        key("c", { ctrlKey: true }),
        { hasSelection: () => false, getSelection: () => "" },
        vi.fn(),
        vi.fn(),
        vi.fn(),
        {},
      ),
    ).toBe(true);
  });
  it.each([{ type: "keyup" }, { repeat: true }])(
    "never repeats app actions %j",
    (overrides) => {
      const execute = vi.fn();
      expect(
        handleKey(
          key("t", { ctrlKey: true, shiftKey: true, ...overrides }),
          { hasSelection: () => false, getSelection: () => "" },
          execute,
          vi.fn(),
          vi.fn(),
          {},
        ),
      ).toBe(false);
      expect(execute).not.toHaveBeenCalled();
    },
  );
});

describe("command palette shortcuts", () => {
  it("returns every default binding for an action", () => {
    expect(shortcutsForAction("Terminal.NewTab")).toEqual([
      "Ctrl+t",
      "Ctrl+Shift+t",
    ]);
  });
  it("reflects disabled and custom bindings", () => {
    expect(
      shortcutsForAction("Terminal.NewTab", {
        "Ctrl+t": "",
        "Ctrl+o": "Terminal.NewTab",
      }),
    ).toEqual(["Ctrl+Shift+t", "Ctrl+o"]);
  });
});
