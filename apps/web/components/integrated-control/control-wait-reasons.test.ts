import { describe, expect, it } from "vitest";
import type { NativeDashboard } from "@/lib/types/control-native";
import { waitReasonCounts } from "./control-wait-reasons";

const dashboard = (overrides: Partial<NativeDashboard> = {}): NativeDashboard => ({
  workspace_id: "workspace-a",
  generated_at: "2026-11-09T12:00:00Z",
  since: "2026-11-08T12:00:00Z",
  basis: "recorded_task_session_queue_transition_state",
  inference_state: "unknown",
  tasks: {},
  sessions: {},
  waiting_input_tasks: 2,
  queued_messages: 0,
  transitions_24h: 0,
  completed_tasks_24h: 0,
  ...overrides,
});

describe("Native waiting reasons", () => {
  it("keeps input independent and gives blocked state precedence over stages", () => {
    expect(
      waitReasonCounts(
        dashboard({
          tasks: { BLOCKED: 8 },
          task_stages: [
            {
              state: "BLOCKED",
              step_id: "approval",
              step_name: "Approve",
              stage_type: "approval",
              count: 1,
            },
            { state: "BLOCKED", step_id: "hold", step_name: "Hold", stage_type: "hold", count: 2 },
            {
              state: "IN_PROGRESS",
              step_id: "approval",
              step_name: "Approve",
              stage_type: "approval",
              count: 3,
            },
            {
              state: "IN_PROGRESS",
              step_id: "hold",
              step_name: "Hold",
              stage_type: "hold",
              count: 4,
            },
            {
              state: "COMPLETED",
              step_id: "approval",
              step_name: "Approve",
              stage_type: "approval",
              count: 5,
            },
            { state: "FAILED", step_id: "hold", step_name: "Hold", stage_type: "hold", count: 6 },
          ],
        }),
      ),
    ).toEqual({ input: 2, blocked: 3, approval: 3, hold: 4 });
  });

  it("distinguishes absent stage data from a measured zero", () => {
    expect(waitReasonCounts(undefined)).toBeUndefined();
    expect(waitReasonCounts(dashboard({ tasks: { BLOCKED: 0 } }))).toEqual({
      input: 2,
      blocked: 0,
      approval: null,
      hold: null,
    });
    expect(waitReasonCounts(dashboard({ task_stages: [] }))).toEqual({
      input: 2,
      blocked: 0,
      approval: 0,
      hold: 0,
    });
  });

  it("marks invalid matching stage counts unknown instead of returning a partial sum", () => {
    expect(
      waitReasonCounts(
        dashboard({
          task_stages: [
            {
              state: "IN_PROGRESS",
              step_id: "approval-1",
              step_name: "A",
              stage_type: "approval",
              count: 2,
            },
            {
              state: "IN_PROGRESS",
              step_id: "approval-2",
              step_name: "B",
              stage_type: "approval",
              count: -1,
            },
          ],
        }),
      ),
    ).toEqual({ input: 2, blocked: 0, approval: null, hold: 0 });
  });
});
