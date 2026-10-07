export type KeyInput = Pick<
  KeyboardEvent,
  | "type"
  | "key"
  | "code"
  | "keyCode"
  | "ctrlKey"
  | "shiftKey"
  | "altKey"
  | "metaKey"
  | "isComposing"
>;

// Ctrl keys that xterm.js already encodes as control characters (space, 3-8, [, \, ]).
const legacyControlKeyCodes = new Set([32, 51, 52, 53, 54, 55, 56, 219, 220, 221]);
// Ctrl+Shift+V pastes; Ctrl+Shift+C is only forwarded once the application asked for
// modifyOtherKeys, since its legacy form (Ctrl+C) would interrupt the running command.
const pasteKey = "v";
const copyKey = "c";
const shiftedDigits = ")!@#$%^&*(";
const symbolKeys: Record<string, [string, string]> = {
  Space: [" ", " "],
  Minus: ["-", "_"],
  Equal: ["=", "+"],
  BracketLeft: ["[", "{"],
  BracketRight: ["]", "}"],
  Backslash: ["\\", "|"],
  Semicolon: [";", ":"],
  Quote: ["'", '"'],
  Comma: [",", "<"],
  Period: [".", ">"],
  Slash: ["/", "?"],
  Backquote: ["`", "~"],
};
const functionKeys: Record<string, string> = {
  F1: "1;%P",
  F2: "1;%Q",
  F3: "1;%R",
  F4: "1;%S",
  F5: "15;%~",
  F6: "17;%~",
  F7: "18;%~",
  F8: "19;%~",
  F9: "20;%~",
  F10: "21;%~",
  F11: "23;%~",
  F12: "24;%~",
};

// Character a physical key produces on a US layout, used to tell Ctrl+Alt chords from AltGr.
export function usLayoutCharacter(code: string, shift: boolean): string {
  if (/^Key[A-Z]$/.test(code)) return shift ? code[3] : code[3].toLowerCase();
  if (/^Digit[0-9]$/.test(code)) return shift ? shiftedDigits[Number(code[5])] : code[5];
  const pair = symbolKeys[code];
  return pair ? pair[shift ? 1 : 0] : "";
}

// Legacy xterm Ctrl+character encoding; characters without a control code are sent as is.
function controlCharacter(character: string): string {
  const upper = character.toUpperCase();
  if (upper >= "A" && upper <= "Z") return String.fromCharCode(upper.charCodeAt(0) - 64);
  const controls: Record<string, string> = {
    " ": "\x00",
    "2": "\x00",
    "@": "\x00",
    "3": "\x1b",
    "[": "\x1b",
    "4": "\x1c",
    "\\": "\x1c",
    "5": "\x1d",
    "]": "\x1d",
    "6": "\x1e",
    "^": "\x1e",
    "7": "\x1f",
    "/": "\x1f",
    "-": "\x1f",
    _: "\x1f",
    "8": "\x7f",
    "?": "\x7f",
  };
  return controls[character] ?? character;
}

function printableCharacter(event: KeyInput): string {
  if (event.key.length === 1) {
    const code = event.key.charCodeAt(0);
    if (code >= 0x20 && code <= 0x7e) return event.key;
  }
  return usLayoutCharacter(event.code, event.shiftKey);
}

// Returns input for unbound key chords that xterm.js drops (e.g. Ctrl+Shift+M, Ctrl+1,
// Ctrl+Alt+A) or flattens (Ctrl+Enter, Ctrl+Tab), or "" to let xterm.js encode the key.
// With modifyOtherKeys enabled by the application (level > 0) chords use the xterm form
// CSI 27 ; modifier ; code ~; otherwise they use xterm's legacy control-character encoding.
// Shift+PageUp/PageDown alone keep scrolling xterm.js scrollback on the normal screen.
export type TerminalKeyState = { alternateScreen?: boolean; bracketedPaste?: boolean };
export function modifyOtherKeysSequence(
  event: KeyInput,
  level: number,
  { alternateScreen = false, bracketedPaste = false }: TerminalKeyState = {},
): string {
  if (event.type !== "keydown" || event.isComposing || event.metaKey) return "";
  const modifier =
    1 + (event.shiftKey ? 1 : 0) + (event.altKey ? 2 : 0) + (event.ctrlKey ? 4 : 0);
  // Ctrl+Backspace: VT line editors (readline, pi, vim), detected by bracketed paste, delete a
  // word with Ctrl+W; Windows console apps (PowerShell, cmd) get xterm's ^H, which ConPTY
  // delivers as Ctrl+Backspace.
  if (
    event.key === "Backspace" &&
    event.ctrlKey &&
    !event.altKey &&
    !event.shiftKey &&
    bracketedPaste
  )
    return "\x17";
  if (
    event.shiftKey &&
    (event.key === "PageUp" || event.key === "PageDown") &&
    (alternateScreen || event.ctrlKey || event.altKey)
  )
    return `\x1b[${event.key === "PageUp" ? 5 : 6};${modifier}~`;
  if (event.ctrlKey && event.altKey && functionKeys[event.key])
    return "\x1b[" + functionKeys[event.key].replace("%", String(modifier));
  if (event.key === "Insert" && (event.altKey || (event.ctrlKey && event.shiftKey)))
    return `\x1b[2;${modifier}~`;
  if (level > 0 && !event.altKey && (event.key === "Enter" || event.key === "Tab")) {
    if (!event.ctrlKey && !(event.key === "Enter" && event.shiftKey)) return "";
    return `\x1b[27;${modifier};${event.key === "Enter" ? 13 : 9}~`;
  }
  if (!event.ctrlKey) return "";
  let character: string;
  if (event.altKey) {
    // Ctrl+Alt is AltGr on Windows: only treat it as a chord when no AltGr character was produced.
    character = usLayoutCharacter(event.code, event.shiftKey);
    if (!character || event.key.toLowerCase() !== character.toLowerCase()) return "";
  } else {
    character = printableCharacter(event);
    if (!character) return "";
    if (
      !event.shiftKey &&
      ((event.keyCode >= 65 && event.keyCode <= 90) ||
        legacyControlKeyCodes.has(event.keyCode))
    )
      return "";
    const lower = character.toLowerCase();
    if (event.shiftKey && (lower === pasteKey || (lower === copyKey && level <= 0)))
      return "";
  }
  if (level > 0) return `\x1b[27;${modifier};${character.charCodeAt(0)}~`;
  const control = controlCharacter(character);
  return event.altKey ? "\x1b" + control : control;
}
