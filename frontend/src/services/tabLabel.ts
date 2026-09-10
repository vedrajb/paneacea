import type { Tab } from "./backend";

export function tabLabel(tab: Tab, index: number): string {
  if (tab.titleMode === "manual" && tab.title.trim()) return tab.title;
  return `Tab ${String(index + 1).padStart(2, "0")}`;
}
