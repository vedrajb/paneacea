import type { Pane } from "./backend";

export function paneHeader(pane: Pane): string {
  const folder =
    pane.currentWorkingDirectory
      .replace(/[/\\]+$/, "")
      .split(/[/\\]/)
      .pop() ||
    pane.currentWorkingDirectory ||
    "Terminal";
  const program =
    (pane.runningProgram || pane.executable).split(/[/\\]/).pop() || "Terminal";
  return `${folder} | ${program}`;
}
