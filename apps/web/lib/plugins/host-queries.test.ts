import { describe, expect, it } from "vitest";
import { hasPluginTaskUsage, projectPluginTaskStatus } from "./host-queries";

describe("projectPluginTaskStatus", () => {
  it("returns the bounded status view used by generic Host plugins", () => {
    expect(
      projectPluginTaskStatus({
        id: "task-1",
        workspace_id: "workspace-1",
        workflow_id: "workflow-1",
        workflow_step_id: "step-2",
        title: "Review migration",
        state: "IN_PROGRESS",
        status_summary: {
          revision: 4,
          updated_at: "2026-09-26T10:00:00Z",
          foreground_activity: "generating",
          active_subagent_count: 2,
          queued_prompt_count: 1,
          completion_gate: {
            revision: 2,
            criteria_count: 3,
            verified_count: 1,
            blocker_count: 2,
            blocked: true,
          },
          active_error: { preview: "test failed", stamp: "event-1", occurred_at: "now" },
          pull_request: { count: 1, url: "https://example.test/1" },
        },
      } as unknown as Parameters<typeof projectPluginTaskStatus>[0]),
    ).toEqual({
      taskId: "task-1",
      title: "Review migration",
      state: "IN_PROGRESS",
      workflowStepId: "step-2",
      statusSummary: {
        revision: 4,
        foregroundActivity: "generating",
        activeSubagentCount: 2,
        queuedPromptCount: 1,
        completionGate: { verifiedCount: 1, criteriaCount: 3, blocked: true },
        activeError: { preview: "test failed", category: undefined },
      },
    });
  });
});

describe("hasPluginTaskUsage", () => {
  const emptyUsage = {
    taskId: "task-1",
    tokensIn: 0,
    tokensCachedRead: 0,
    tokensCachedWrite: 0,
    tokensOut: 0,
    tokensThought: 0,
    tokensTotal: 0,
    costSubcents: 0,
    eventCount: 0,
    estimatedEventCount: 0,
    unpricedEventCount: 0,
    outputTokensComplete: true,
  } as const;

  it("does not present an unobserved task as free usage", () => {
    expect(hasPluginTaskUsage(emptyUsage)).toBe(false);
  });

  it("retains measured, estimated, and unpriced usage observations", () => {
    expect(hasPluginTaskUsage({ ...emptyUsage, eventCount: 1 })).toBe(true);
    expect(hasPluginTaskUsage({ ...emptyUsage, estimatedEventCount: 1 })).toBe(true);
    expect(hasPluginTaskUsage({ ...emptyUsage, unpricedEventCount: 1 })).toBe(true);
  });
});
