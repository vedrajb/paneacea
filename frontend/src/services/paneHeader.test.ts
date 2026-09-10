import { expect, test } from "vitest";
import type { Pane } from "./backend";
import { paneHeader } from "./paneHeader";

test("header uses the folder and running executable instead of the terminal title", () => {
  const pane = {
    currentWorkingDirectory: "C:\\dev\\project\\",
    executable: "C:\\Windows\\powershell.exe",
    runningProgram: "C:\\tools\\node.exe",
    title: "Unrelated title",
  } as Pane;
  expect(paneHeader(pane)).toBe("project | node.exe");
  pane.runningProgram = "";
  expect(paneHeader(pane)).toBe("project | powershell.exe");
  pane.currentWorkingDirectory = "C:\\";
  expect(paneHeader(pane)).toBe("C: | powershell.exe");
});
