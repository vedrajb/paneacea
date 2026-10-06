type AgentInfo = { type: string; state?: string; resumeCommand?: string };
type PaneInfo = { status?: string; busy?: boolean; agent?: AgentInfo | null };

// States reported while the agent process is still alive.
const runningStates = new Set(["unknown", "working", "waiting"]);

// Fallback unlock if the resumed agent is never seen running (e.g. the user
// quits the picker immediately). Matches resumeLockDuration in the runtime.
export const RESUME_LOCK_MS = 10_000;

export function agentIsRunning(agent?: AgentInfo | null): boolean {
  return !!agent && runningStates.has(agent.state ?? "unknown");
}

/**
 * True when clicking the icon can resume the pane's last agent: the agent has a
 * resume command and no command (agent or otherwise) is running in the pane.
 */
export function paneAgentResumable(
  pane?: PaneInfo | null,
  resuming = false,
): boolean {
  const agent = pane?.agent;
  return (
    !resuming &&
    !!agent?.type &&
    !!agent.resumeCommand &&
    pane?.status === "running" &&
    !pane.busy &&
    !agentIsRunning(agent)
  );
}

/** Hover text: the agent's resume command, or its type if it has none. */
export function paneAgentTitle(agent?: AgentInfo | null): string {
  if (!agent || !agent.type) return "";
  return agent.resumeCommand || agent.type;
}
