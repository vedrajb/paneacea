import { describe, expect, it, vi } from "vitest";
import { handleKey, resolve } from "./actions";

function key(overrides: Partial<KeyboardEvent> = {}): KeyboardEvent {
  return {
    key: "b",
    type: "keydown",
    code: "KeyB",
    ctrlKey: true,
    shiftKey: false,
    altKey: false,
    metaKey: false,
    isComposing: false,
    repeat: false,
    preventDefault: vi.fn(),
    ...overrides,
  } as unknown as KeyboardEvent;
}

describe("workspace selector shortcut", () => {
  it("maps Ctrl+B to the workspace selector", () => {
    expect(resolve(key())).toBe("Workspace.OpenSelector");
  });

  it("executes the workspace selector action", () => {
    const execute = vi.fn();
    const event = key();

    expect(
      handleKey(
        event,
        { hasSelection: () => false, getSelection: () => "" },
        execute,
        vi.fn(),
        vi.fn(),
        {},
      ),
    ).toBe(false);
    expect(execute).toHaveBeenCalledWith("Workspace.OpenSelector");
    expect(event.preventDefault).toHaveBeenCalledOnce();
  });
});
