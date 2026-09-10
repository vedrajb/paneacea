export function visiblePaneState(status: string, agentState?: string): string {
  if (agentState) return agentState;
  return status === "error" ? status : "";
}
