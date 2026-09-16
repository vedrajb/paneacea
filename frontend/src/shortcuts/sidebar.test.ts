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

describe("removed side panel shortcut", () => {
  it("leaves Ctrl+B unbound", () => {
    expect(resolve(key())).toBeNull();
  });

  it("does not execute an action", () => {
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
    ).toBe(true);
    expect(execute).not.toHaveBeenCalled();
    expect(event.preventDefault).not.toHaveBeenCalled();
  });
});
