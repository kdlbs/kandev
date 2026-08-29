import { describe, expect, it } from "vitest";
import { buildSidebarItem } from "./task-session-sidebar-item";

type SidebarTask = Parameters<typeof buildSidebarItem>[0];
type SidebarContext = Parameters<typeof buildSidebarItem>[1];
const UPDATED_AT = "2026-07-22T00:00:00Z";
const ACTIVITY_AT = "2026-07-24T00:00:00Z";
const CREATED_AT = "2026-07-20T00:00:00Z";
const SUMMARY_UPDATED_AT = "2026-07-25T00:00:00Z";

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

function repositoryProjectionTask(): SidebarTask {
  return task({
    repositoryId: "repo-a",
    repositories: [
      { id: "link-b", repository_id: "repo-b", base_branch: "main", position: 2 },
      { id: "link-a", repository_id: "repo-a", base_branch: "main", position: 1 },
      {
        id: "link-a-duplicate",
        repository_id: "repo-a",
        base_branch: "main",
        position: 3,
      },
    ],
  });
}

function repositoryProjectionContext(): SidebarContext {
  return {
    ...emptyContext(),
    repositorySlugById: new Map([
      ["repo-a", "owner/repo-a"],
      ["repo-b", "owner/repo-b"],
    ]),
  };
}

describe("buildSidebarItem", () => {
  it("projects unique repository slugs in attachment order", () => {
    const item = buildSidebarItem(repositoryProjectionTask(), repositoryProjectionContext());

    expect(item.repositories).toEqual(["owner/repo-a", "owner/repo-b"]);
    expect(item.repositoryLinks).toHaveLength(3);
  });

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

  it("preserves the archived marker for projected rows", () => {
    const item = buildSidebarItem(task({ isArchived: true }), emptyContext());

    expect(item.isArchived).toBe(true);
  });

  it("carries the queued prompt count from the status summary", () => {
    const item = buildSidebarItem(
      task({ statusSummary: { revision: 4, updated_at: UPDATED_AT, queued_prompt_count: 3 } }),
      emptyContext(),
    );

    expect(item.queuedCount).toBe(3);
  });

  it("leaves queuedCount undefined when the summary has no queued prompts", () => {
    const item = buildSidebarItem(
      task({ statusSummary: { revision: 5, updated_at: UPDATED_AT } }),
      emptyContext(),
    );

    expect(item.queuedCount).toBeUndefined();
  });

  it("carries WIP queue status separately from queued prompts", () => {
    const wipQueue = { position: 2, total: 4, destinationTitle: "Review" };
    const item = buildSidebarItem(task(), {
      ...emptyContext(),
      wipQueueByTaskId: new Map([["t1", wipQueue]]),
    });

    expect(item.wipQueue).toEqual(wipQueue);
    expect(item.queuedCount).toBeUndefined();
  });
});

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
describe("buildSidebarItem automation indicators", () => {
  it("carries bounded automation indicators into the row PR info", () => {
    const item = buildSidebarItem(
      task({
        statusSummary: {
          revision: 2,
          updated_at: UPDATED_AT,
          pull_request: {
            number: 42,
            state: "open",
            auto_fix_enabled: true,
            auto_merge_enabled: true,
          },
        },
      }),
      emptyContext(),
    );

    expect(item.prInfo).toEqual({
      number: 42,
      state: "Open",
      autoFixEnabled: true,
      autoMergeEnabled: true,
    });
  });
});

describe("buildSidebarItem activity", () => {
  it("maps summary activity and falls back through task update to creation", () => {
    const projected = buildSidebarItem(
      task({
        updatedAt: UPDATED_AT,
        createdAt: CREATED_AT,
        statusSummary: {
          revision: 6,
          updated_at: SUMMARY_UPDATED_AT,
          last_activity_at: ACTIVITY_AT,
        },
      }),
      emptyContext(),
    );
    expect(projected.updatedAt).toBe(SUMMARY_UPDATED_AT);
    expect(projected.lastActivityAt).toBe(ACTIVITY_AT);

    const updatedFallback = buildSidebarItem(
      task({
        updatedAt: UPDATED_AT,
        createdAt: CREATED_AT,
        statusSummary: { revision: 7, updated_at: SUMMARY_UPDATED_AT },
      }),
      emptyContext(),
    );
    expect(updatedFallback.lastActivityAt).toBe(UPDATED_AT);

    const createdFallback = buildSidebarItem(
      task({
        createdAt: CREATED_AT,
        statusSummary: { revision: 8, updated_at: SUMMARY_UPDATED_AT },
      }),
      emptyContext(),
    );
    expect(createdFallback.lastActivityAt).toBe(CREATED_AT);
  });
});
