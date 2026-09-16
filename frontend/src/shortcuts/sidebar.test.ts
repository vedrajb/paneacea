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

describe("side panel shortcut", () => {
  it("binds Ctrl+B to toggling the side panel", () => {
    expect(resolve(key())).toBe("Paneacea.ToggleSidebar");
  });

  it("executes the toggle once per key press", () => {
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
    expect(execute).toHaveBeenCalledExactlyOnceWith(
      "Paneacea.ToggleSidebar",
    );
    expect(event.preventDefault).toHaveBeenCalledOnce();
  });
});
