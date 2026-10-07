import { describe, expect, it } from "vitest";
import {
  modifyOtherKeysSequence,
  usLayoutCharacter,
  type KeyInput,
} from "./modifyOtherKeys";

const key = (overrides: Partial<KeyInput>): KeyInput => ({
  type: "keydown",
  key: "M",
  code: "KeyM",
  keyCode: 77,
  ctrlKey: true,
  shiftKey: true,
  altKey: false,
  metaKey: false,
  isComposing: false,
  ...overrides,
});
const ctrl = (value: string, code: string, keyCode: number) =>
  key({ key: value, code, keyCode, shiftKey: false });

describe("modifyOtherKeysSequence", () => {
  it("encodes Ctrl+Shift+letter like xterm once enabled", () => {
    expect(modifyOtherKeysSequence(key({}), 2)).toBe("\x1b[27;6;77~");
  });

  it("falls back to legacy control characters without modifyOtherKeys", () => {
    expect(modifyOtherKeysSequence(key({}), 0)).toBe("\r");
    expect(modifyOtherKeysSequence(key({ key: "A", code: "KeyA", keyCode: 65 }), 0)).toBe("\x01");
    expect(modifyOtherKeysSequence(ctrl("/", "Slash", 191), 0)).toBe("\x1f");
    expect(modifyOtherKeysSequence(ctrl("2", "Digit2", 50), 0)).toBe("\x00");
    expect(modifyOtherKeysSequence(key({ key: " ", code: "Space", keyCode: 32 }), 0)).toBe("\x00");
  });

  it("sends the character for Ctrl chords with no control code, as xterm does", () => {
    expect(modifyOtherKeysSequence(ctrl("1", "Digit1", 49), 0)).toBe("1");
    expect(modifyOtherKeysSequence(ctrl(".", "Period", 190), 0)).toBe(".");
    expect(modifyOtherKeysSequence(key({ key: "!", code: "Digit1", keyCode: 49 }), 0)).toBe("!");
  });

  it("encodes Ctrl+digit and Ctrl+symbol chords once enabled", () => {
    expect(modifyOtherKeysSequence(ctrl("1", "Digit1", 49), 2)).toBe("\x1b[27;5;49~");
    expect(modifyOtherKeysSequence(ctrl(";", "Semicolon", 186), 2)).toBe("\x1b[27;5;59~");
  });

  it("uses the physical key for non-Latin layouts", () => {
    expect(modifyOtherKeysSequence(key({ key: "Ь" }), 2)).toBe("\x1b[27;6;77~");
  });

  it("encodes modified Enter and Tab once enabled", () => {
    const enter = { key: "Enter", code: "Enter", keyCode: 13 };
    expect(modifyOtherKeysSequence(key({ ...enter, shiftKey: false }), 2)).toBe("\x1b[27;5;13~");
    expect(modifyOtherKeysSequence(key({ ...enter, ctrlKey: false }), 2)).toBe("\x1b[27;2;13~");
    expect(modifyOtherKeysSequence(key({ key: "Tab", code: "Tab", keyCode: 9 }), 2)).toBe("\x1b[27;6;9~");
    expect(modifyOtherKeysSequence(key({ key: "Tab", code: "Tab", keyCode: 9, ctrlKey: false }), 2)).toBe("");
    expect(modifyOtherKeysSequence(key({ ...enter, ctrlKey: false, shiftKey: false }), 2)).toBe("");
    expect(modifyOtherKeysSequence(key({ ...enter, shiftKey: false }), 0)).toBe("");
  });

  it("encodes Ctrl+Alt chords that xterm.js treats as AltGr on Windows", () => {
    const ctrlAltA = key({ key: "a", code: "KeyA", keyCode: 65, shiftKey: false, altKey: true });
    expect(modifyOtherKeysSequence(ctrlAltA, 0)).toBe("\x1b\x01");
    expect(modifyOtherKeysSequence(ctrlAltA, 2)).toBe("\x1b[27;7;97~");
    expect(
      modifyOtherKeysSequence(key({ key: "1", code: "Digit1", keyCode: 49, shiftKey: false, altKey: true }), 0),
    ).toBe("\x1b1");
  });

  it("leaves AltGr characters to xterm.js", () => {
    expect(
      modifyOtherKeysSequence(key({ key: "@", code: "KeyQ", keyCode: 81, shiftKey: false, altKey: true }), 2),
    ).toBe("");
    expect(
      modifyOtherKeysSequence(key({ key: "[", code: "Digit8", keyCode: 56, shiftKey: false, altKey: true }), 0),
    ).toBe("");
  });

  it("encodes Ctrl+Alt function keys and modified Insert", () => {
    expect(modifyOtherKeysSequence(key({ key: "F1", code: "F1", keyCode: 112, shiftKey: false, altKey: true }), 0)).toBe("\x1b[1;7P");
    expect(modifyOtherKeysSequence(key({ key: "F5", code: "F5", keyCode: 116, altKey: true }), 0)).toBe("\x1b[15;8~");
    expect(modifyOtherKeysSequence(key({ key: "Insert", code: "Insert", keyCode: 45, ctrlKey: false, altKey: true }), 0)).toBe("\x1b[2;4~");
    expect(modifyOtherKeysSequence(key({ key: "Insert", code: "Insert", keyCode: 45 }), 0)).toBe("\x1b[2;6~");
  });

  it("leaves chords xterm.js already encodes to xterm.js", () => {
    expect(modifyOtherKeysSequence(ctrl("m", "KeyM", 77), 2)).toBe("");
    expect(modifyOtherKeysSequence(ctrl("3", "Digit3", 51), 2)).toBe("");
    expect(modifyOtherKeysSequence(ctrl("[", "BracketLeft", 219), 2)).toBe("");
    expect(modifyOtherKeysSequence(key({ key: "Insert", code: "Insert", keyCode: 45, shiftKey: false }), 0)).toBe("");
    expect(modifyOtherKeysSequence(key({ key: "a", code: "KeyA", keyCode: 65, ctrlKey: false, shiftKey: false, altKey: true }), 0)).toBe("");
  });

  it("keeps Ctrl+Shift+V for paste and forwards Ctrl+Shift+C only when requested", () => {
    for (const level of [0, 2])
      expect(modifyOtherKeysSequence(key({ key: "V", code: "KeyV", keyCode: 86 }), level)).toBe("");
    const copy = key({ key: "C", code: "KeyC", keyCode: 67 });
    expect(modifyOtherKeysSequence(copy, 0)).toBe("");
    expect(modifyOtherKeysSequence(copy, 2)).toBe("\x1b[27;6;67~");
  });

  it("forwards Shift+PageUp/PageDown unless they scroll normal-screen scrollback", () => {
    const pageUp = key({ key: "PageUp", code: "PageUp", keyCode: 33, ctrlKey: false });
    expect(modifyOtherKeysSequence(pageUp, 0)).toBe("");
    expect(modifyOtherKeysSequence(pageUp, 0, { alternateScreen: true })).toBe("\x1b[5;2~");
    expect(
      modifyOtherKeysSequence(key({ key: "PageDown", code: "PageDown", keyCode: 34 }), 0),
    ).toBe("\x1b[6;6~");
  });

  it("deletes a word with Ctrl+Backspace in VT line editors and leaves ^H for console apps", () => {
    const ctrlBackspace = key({ key: "Backspace", code: "Backspace", keyCode: 8, shiftKey: false });
    expect(modifyOtherKeysSequence(ctrlBackspace, 0, { bracketedPaste: true })).toBe("\x17");
    expect(modifyOtherKeysSequence(ctrlBackspace, 2, { bracketedPaste: true })).toBe("\x17");
    expect(modifyOtherKeysSequence(ctrlBackspace, 0)).toBe("");
    expect(
      modifyOtherKeysSequence(key({ key: "Backspace", code: "Backspace", keyCode: 8, shiftKey: false, altKey: true }), 0, { bracketedPaste: true }),
    ).toBe("");
  });

  it("ignores Meta, key releases and composition", () => {
    expect(modifyOtherKeysSequence(key({ metaKey: true }), 2)).toBe("");
    expect(modifyOtherKeysSequence(key({ type: "keyup" }), 2)).toBe("");
    expect(modifyOtherKeysSequence(key({ isComposing: true }), 2)).toBe("");
  });
});

describe("usLayoutCharacter", () => {
  it("maps physical keys to US layout characters", () => {
    expect(usLayoutCharacter("KeyM", false)).toBe("m");
    expect(usLayoutCharacter("KeyM", true)).toBe("M");
    expect(usLayoutCharacter("Digit2", true)).toBe("@");
    expect(usLayoutCharacter("Slash", true)).toBe("?");
    expect(usLayoutCharacter("ArrowUp", false)).toBe("");
  });
});
