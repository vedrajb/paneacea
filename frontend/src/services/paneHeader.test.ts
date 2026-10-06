import { expect, test } from "vitest";
import type { Pane } from "./backend";
import { paneHeader } from "./paneHeader";

test("header shows only the current path, ignoring the running program and title", () => {
  const pane = {
    currentWorkingDirectory: "C:\\dev\\project\\",
    executable: "C:\\Windows\\powershell.exe",
    runningProgram: "C:\\tools\\node.exe",
    title: "Unrelated title",
  } as Pane;
  expect(paneHeader(pane)).toBe("C:\\dev\\project\\");
  pane.currentWorkingDirectory = "C:\\";
  expect(paneHeader(pane)).toBe("C:\\");
  pane.currentWorkingDirectory = "";
  expect(paneHeader(pane)).toBe("Terminal");
});
