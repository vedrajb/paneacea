import { expect, test } from "vitest";
import {
  terminalCursorOptions,
  terminalTheme,
} from "../services/terminalOptions";

test("uses a bar cursor when a terminal is unfocused", () => {
  expect(terminalCursorOptions.cursorStyle).toBe("bar");
  expect(terminalCursorOptions.cursorInactiveStyle).toBe("bar");
  expect(terminalCursorOptions.cursorWidth).toBe(2);
});

test("uses the darker Windows Terminal-style palette", () => {
  expect(terminalTheme.background).toBe("#0c0c0c");
  expect(terminalTheme.foreground).toBe("#cccccc");
  expect(terminalTheme.brightBlue).toBe("#3b78ff");
});
