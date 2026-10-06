import { expect, test } from "vitest";
import {
  agentIsRunning,
  paneAgentResumable,
  paneAgentTitle,
} from "./paneAgent";

const codex = { type: "codex", state: "done", resumeCommand: "codex resume" };
const idlePane = { status: "running", busy: false, agent: codex };

test("returns empty title without an agent", () => {
  expect(paneAgentTitle()).toBe("");
  expect(paneAgentTitle(null)).toBe("");
  expect(paneAgentTitle({ type: "" })).toBe("");
});

test("hover shows only the resume command", () => {
  expect(paneAgentTitle(codex)).toBe("codex resume");
  expect(
    paneAgentTitle({
      type: "claude",
      state: "unknown",
      resumeCommand: "claude --resume",
    }),
  ).toBe("claude --resume");
  expect(
    paneAgentTitle({ type: "pi", state: "done", resumeCommand: "pi --resume" }),
  ).toBe("pi --resume");
  expect(paneAgentTitle({ type: "gemini", state: "done" })).toBe("gemini");
});

test("clickable only when no command is running in the pane", () => {
  expect(paneAgentResumable(idlePane)).toBe(true);
  expect(paneAgentResumable({ ...idlePane, busy: true })).toBe(false);
  expect(
    paneAgentResumable({ ...idlePane, agent: { ...codex, state: "working" } }),
  ).toBe(false);
  expect(paneAgentResumable({ ...idlePane, status: "exited" })).toBe(false);
  expect(paneAgentResumable(idlePane, true)).toBe(false);
});

test("agents without a resume command are not clickable", () => {
  expect(
    paneAgentResumable({
      ...idlePane,
      agent: { type: "gemini", state: "done" },
    }),
  ).toBe(false);
  expect(paneAgentResumable({ status: "running" })).toBe(false);
  expect(paneAgentResumable(null)).toBe(false);
});

test("missing state counts as running", () => {
  expect(agentIsRunning({ type: "codex" })).toBe(true);
  expect(agentIsRunning(null)).toBe(false);
});
