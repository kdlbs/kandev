import { describe, expect, it } from "vitest";
import { buildSidebarItem } from "./task-session-sidebar-item";

type SidebarTask = Parameters<typeof buildSidebarItem>[0];
type SidebarContext = Parameters<typeof buildSidebarItem>[1];
const UPDATED_AT = "2026-07-22T00:00:00Z";

function emptyContext(): SidebarContext {
  return {
    repositorySlugById: new Map(),
    titleById: new Map(),
    workflowNameById: new Map(),
    stepTitleById: new Map(),
  };
}

function task(overrides: Partial<SidebarTask> = {}): SidebarTask {
  return {
    id: "t1",
    _workflowId: "wf1",
    title: "Task",
    workflowStepId: "step-1",
    ...overrides,
  } as SidebarTask;
}

describe("buildSidebarItem", () => {
  it("keeps the PR aggregate state for the row icon", () => {
    const item = buildSidebarItem(
      task({
        statusSummary: {
          revision: 1,
          updated_at: UPDATED_AT,
          pull_request: {
            number: 42,
            state: "open",
            aggregate_state: "pending",
          },
        },
      }),
      emptyContext(),
    );

    expect(item.prInfo).toEqual({ number: 42, state: "Open", aggregateState: "pending" });
  });

  it("uses summary presence as the authority for cleared session and pending fields", () => {
    const item = buildSidebarItem(
      task({
        taskPendingAction: "clarification",
        primarySessionState: "RUNNING",
        primarySessionId: "legacy-session",
        foregroundActivity: "background",
        updatedAt: "legacy-update",
        statusSummary: { revision: 2, updated_at: UPDATED_AT },
      }),
      emptyContext(),
    );

    expect(item.hasPendingClarification).toBe(false);
    expect(item.sessionState).toBeUndefined();
    expect(item.primarySessionId).toBeNull();
    expect(item.foregroundActivity).toBeUndefined();
    expect(item.updatedAt).toBe(UPDATED_AT);
  });

  it("honors the summary error acknowledgement stamp", () => {
    const item = buildSidebarItem(
      task({
        statusSummary: {
          revision: 3,
          updated_at: UPDATED_AT,
          active_error: {
            session_id: "session-1",
            stamp: "error-3",
            occurred_at: UPDATED_AT,
            preview: "Agent failed",
          },
        },
      }),
      {
        ...emptyContext(),
        acknowledgedAgentErrors: { "session-1": "error-3" },
      },
    );

    expect(item.agentErrorMessage).toBeNull();
  });
});
