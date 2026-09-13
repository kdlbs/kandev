import { describe, expect, it } from "vitest";
import { mergeAgentGoalMetadata, parseAgentGoal, type AgentGoal } from "./agent-goal";

function goal(status: AgentGoal["status"], createdAt = 10, updatedAt = 20): AgentGoal {
  return { objective: "Coordinate contributor PR reviews", status, createdAt, updatedAt };
}

describe("agent goal projection", () => {
  it("accepts each supported provider status", () => {
    for (const status of ["active", "paused", "blocked", "limited", "complete"] as const) {
      expect(parseAgentGoal(goal(status))?.status).toBe(status);
    }
  });

  it("retains an accepted goal across sparse metadata and explicit clear", () => {
    const active = { goal: goal("active"), provider: "codex" };

    expect(mergeAgentGoalMetadata(active, { codex: { threadStatus: "idle" } })).toEqual({
      codex: { threadStatus: "idle" },
      goal: goal("active"),
    });
    expect(mergeAgentGoalMetadata(active, { goal: null })).toEqual({ goal: null });
  });

  it("rejects malformed and older snapshots without making them active", () => {
    const active = { goal: goal("active", 10, 20) };

    expect(parseAgentGoal({ ...goal("active"), status: "running" })).toBeNull();
    expect(mergeAgentGoalMetadata(active, { goal: goal("active", 10, 19) })).toEqual(active);
  });
});
