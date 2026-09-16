import { expect, test } from "vitest";
import { terminalCursorOptions } from "../services/terminalOptions";

test("uses a bar cursor when a terminal is unfocused", () => {
  expect(terminalCursorOptions.cursorStyle).toBe("bar");
  expect(terminalCursorOptions.cursorInactiveStyle).toBe("bar");
  expect(terminalCursorOptions.cursorWidth).toBe(2);
});
