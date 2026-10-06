import type { Pane } from "./backend";

export function paneHeader(pane: Pane): string {
  return pane.currentWorkingDirectory || "Terminal";
}
