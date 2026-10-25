import { describe, expect, it } from "vitest";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { selectThreadCandidates } from "@/lib/threads/thread-view-query";

/**
 * AC-UI-NEEDS-YOU-INBOX-001.27: given a fixture where the only pending action
 * on every task is an answerable clarification bundle, the Inbox row set and
 * the Threads needs-action preset name the same tasks. The two membership
 * predicates are otherwise independent by design — Threads derives
 * `needs_action` from session/task pending-action fields
 * (`lib/threads/active-threads.ts`), while the Inbox row set is the backend
 * bundle query's answerability predicate
 * (`apps/backend/internal/task/repository/sqlite/clarification_bundle_query.go`).
 * This file asserts the Threads half of each named divergence: a fixture the
 * Inbox structurally cannot or must not list still reads `needs_action` here,
 * because Threads has no field carrying the excluding fact. The Inbox-side
 * exclusion for each case is covered by the backend's own bundle-query tests.
 */

type TaskOverrides = Partial<KanbanState["tasks"][number]> & { id: string };

function task(overrides: TaskOverrides): KanbanState["tasks"][number] {
  const { id, ...rest } = overrides;
  return {
    id,
    workspaceId: "workspace-1",
    workflowId: "workflow-1",
    workflowStepId: "step-build",
    title: `Task ${id}`,
    position: 0,
    state: "IN_PROGRESS",
    primarySessionId: `session-${id}`,
    primarySessionState: "RUNNING",
    updatedAt: "2026-08-31T10:00:00Z",
    createdAt: "2026-08-30T10:00:00Z",
    ...rest,
  };
}

function snapshot(tasks: KanbanState["tasks"]): Record<string, WorkflowSnapshotData> {
  return {
    "workflow-1": {
      workflowId: "workflow-1",
      workflowName: "Delivery",
      steps: [{ id: "step-build", title: "Build", color: "blue", position: 0 }],
      tasks,
    },
  };
}

function needsActionStatuses(tasks: KanbanState["tasks"]) {
  return new Map(
    selectThreadCandidates(snapshot(tasks)).map((candidate) => [
      candidate.taskId,
      candidate.threadStatus,
    ]),
  );
}

describe("Needs-you Inbox and Threads needs-action agreement (AC .27)", () => {
  it("coincides on an answerable clarification bundle: waiting session, clarification pending action", () => {
    const statuses = needsActionStatuses([
      task({
        id: "coincide",
        primarySessionState: "WAITING_FOR_INPUT",
        primarySessionPendingAction: "clarification",
      }),
    ]);

    expect(statuses.get("coincide")).toBe("needs_action");
  });

  it("diverges on a pending tool-permission request: Threads reads needs_action, the bundle path never returns permission requests", () => {
    const statuses = needsActionStatuses([
      task({
        id: "permission",
        primarySessionState: "WAITING_FOR_INPUT",
        primarySessionPendingAction: "permission",
      }),
    ]);

    expect(statuses.get("permission")).toBe("needs_action");
  });

  it("diverges on a terminal session: Threads has no terminal check and still reads needs_action from a stale pending action", () => {
    const statuses = needsActionStatuses([
      task({
        id: "terminal",
        primarySessionState: "COMPLETED",
        primarySessionPendingAction: "clarification",
      }),
    ]);

    expect(statuses.get("terminal")).toBe("needs_action");
  });

  it("diverges silently on a parent-question record and on a bundle with no resolvable question identifier: Threads carries neither fact, so both fixtures are indistinguishable from the coincide case here", () => {
    const statuses = needsActionStatuses([
      task({
        id: "parent-question-or-unresolvable",
        primarySessionState: "WAITING_FOR_INPUT",
        primarySessionPendingAction: "clarification",
      }),
    ]);

    expect(statuses.get("parent-question-or-unresolvable")).toBe("needs_action");
  });
});
