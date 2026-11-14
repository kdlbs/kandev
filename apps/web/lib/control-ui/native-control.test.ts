import { describe, expect, it } from "vitest";
import { nativeDashboard } from "./native-control";

const validDashboard = {
  workspace_id: "workspace-a",
  generated_at: "2026-11-09T12:00:00Z",
  since: "2026-11-08T12:00:00Z",
  basis: "recorded_task_session_queue_transition_state",
  inference_state: "unknown",
  tasks: { IN_PROGRESS: 2 },
  sessions: { RUNNING: 1 },
  waiting_input_tasks: 0,
  queued_messages: 0,
  transitions_24h: 0,
  completed_tasks_24h: 0,
  task_stages: [],
};

describe("native control dashboard contract", () => {
  it("accepts a workspace-scoped read with an exact 24-hour window", () => {
    expect(nativeDashboard(validDashboard, "workspace-a")).toBe(true);
  });

  it.each([
    ["another workspace", { ...validDashboard, workspace_id: "workspace-b" }],
    ["a short window", { ...validDashboard, since: "2026-11-08T13:00:00Z" }],
    ["an invalid timestamp", { ...validDashboard, generated_at: "2026-11-09T12:00:00" }],
    ["a changed basis", { ...validDashboard, basis: "inferred_task_state" }],
    ["a known inference value", { ...validDashboard, inference_state: "complete" }],
    ["a negative count", { ...validDashboard, tasks: { IN_PROGRESS: -1 } }],
    ["an unsafe count", { ...validDashboard, waiting_input_tasks: Number.MAX_SAFE_INTEGER + 1 }],
    ["a malformed stage row", { ...validDashboard, task_stages: [{ state: "BLOCKED", count: 1 }] }],
  ])("rejects %s", (_name, value) => {
    expect(nativeDashboard(value, "workspace-a")).toBe(false);
  });
});
