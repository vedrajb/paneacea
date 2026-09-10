import type { Pane } from "./backend";

export function countPanesForTab(
  panes: Record<string, Pick<Pane, "tabId">>,
  tabId: string,
): number {
  return Object.values(panes).filter((pane) => pane.tabId === tabId).length;
}
