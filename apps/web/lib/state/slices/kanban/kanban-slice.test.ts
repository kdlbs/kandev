import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createKanbanSlice } from "./kanban-slice";
import type { KanbanSlice } from "./types";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-a";
const ARCHIVED_WORKSPACE_ID = "workspace-a";
const AUTO_SESSION_ID = "session-auto";
const PINNED_SESSION_ID = "session-pinned";

function makeStore() {
  return create<KanbanSlice>()(immer(createKanbanSlice));
}

describe("kanban slice active session selection", () => {
  it("updates active session state without creating a user pin", () => {
    const store = makeStore();

    store.getState().setActiveSessionAuto(TASK_ID, AUTO_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: TASK_ID,
      activeSessionId: AUTO_SESSION_ID,
      pinnedSessionId: null,
      lastSessionByTaskId: { [TASK_ID]: AUTO_SESSION_ID },
    });
  });

  it("preserves an existing pin when auto-selecting the pinned session", () => {
    const store = makeStore();

    store.getState().setActiveSession(TASK_ID, PINNED_SESSION_ID);
    store.getState().setActiveSessionAuto(TASK_ID, PINNED_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: TASK_ID,
      activeSessionId: PINNED_SESSION_ID,
      pinnedSessionId: PINNED_SESSION_ID,
      lastSessionByTaskId: { [TASK_ID]: PINNED_SESSION_ID },
    });
  });

  it("leaves non-matching pins for callers to resolve", () => {
    const store = makeStore();

    store.getState().setActiveSession(TASK_ID, PINNED_SESSION_ID);
    store.getState().setActiveSessionAuto(TASK_ID, AUTO_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: TASK_ID,
      activeSessionId: AUTO_SESSION_ID,
      pinnedSessionId: PINNED_SESSION_ID,
      lastSessionByTaskId: { [TASK_ID]: AUTO_SESSION_ID },
    });
  });

  it("clears a pin when auto-selecting a session for a different task", () => {
    const store = makeStore();

    store.getState().setActiveSession(TASK_ID, PINNED_SESSION_ID);
    store.getState().setActiveSessionAuto("task-2", AUTO_SESSION_ID);

    expect(store.getState().tasks).toMatchObject({
      activeTaskId: "task-2",
      activeSessionId: AUTO_SESSION_ID,
      pinnedSessionId: null,
      lastSessionByTaskId: { [TASK_ID]: PINNED_SESSION_ID, "task-2": AUTO_SESSION_ID },
    });
  });
});

// eslint-disable-next-line max-lines-per-function -- workspace transition cases share one store harness
describe("kanban slice workspace transition", () => {
  it("clears workflow, board, and active task context before loading another workspace", () => {
    const store = makeStore();
    store.setState({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [{ id: "step-a", title: "Todo", color: "blue", position: 0 }],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: "step-a",
            title: "Workspace A task",
            position: 0,
          },
        ],
      },
      kanbanMulti: {
        snapshots: {
          [WORKFLOW_ID]: {
            workflowId: WORKFLOW_ID,
            workflowName: "Workflow A",
            steps: [],
            tasks: [],
          },
        },
        isLoading: true,
        orderRevisionByStepId: {},
        pendingReorderBandKeys: {},
        withheldReorderByBandKey: {},
      },
      workflows: {
        items: [{ id: WORKFLOW_ID, workspaceId: ARCHIVED_WORKSPACE_ID, name: "Workflow A" }],
        activeId: WORKFLOW_ID,
      },
      workspaceContextGeneration: 7,
      tasks: {
        activeTaskId: TASK_ID,
        activeSessionId: PINNED_SESSION_ID,
        pinnedSessionId: PINNED_SESSION_ID,
        lastSessionByTaskId: { [TASK_ID]: PINNED_SESSION_ID },
        resumeSkippedSessionIds: {},
      },
    });

    store.getState().resetKanbanWorkspaceContext();

    expect(store.getState()).toMatchObject({
      kanban: { workflowId: null, steps: [], tasks: [] },
      kanbanMulti: { snapshots: {}, isLoading: false },
      workflows: { items: [], activeId: null },
      workspaceContextGeneration: 8,
      tasks: {
        activeTaskId: null,
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
  });

  it("tracks scoped context read outcomes and ignores stale generations", () => {
    const store = makeStore();

    store.getState().setWorkspaceContextRead("workflows", ARCHIVED_WORKSPACE_ID, 0, "pending");
    store
      .getState()
      .setWorkspaceContextRead("workflows", ARCHIVED_WORKSPACE_ID, 0, "transient", 4_000);
    expect(store.getState().workspaceContextRead).toMatchObject({
      workspaceId: ARCHIVED_WORKSPACE_ID,
      generation: 0,
      pending: { workflows: false },
      errors: { workflows: "transient" },
      retryAfterMs: { workflows: 4_000 },
    });

    store.getState().resetKanbanWorkspaceContext();
    store.getState().setWorkspaceContextRead("workflows", ARCHIVED_WORKSPACE_ID, 0, "success");
    expect(store.getState().workspaceContextRead).toMatchObject({
      workspaceId: null,
      generation: 0,
      errors: { workflows: null },
    });
    expect(store.getState().workspaceContextGeneration).toBe(1);
  });

  it("clears a context error on success and preserves the retry version", () => {
    const store = makeStore();

    store.getState().requestWorkspaceContextRefresh();
    store.getState().setWorkspaceContextRead("repositories", ARCHIVED_WORKSPACE_ID, 0, "pending");
    store
      .getState()
      .setWorkspaceContextRead("repositories", ARCHIVED_WORKSPACE_ID, 0, "access_denied");
    store.getState().setWorkspaceContextRead("repositories", ARCHIVED_WORKSPACE_ID, 0, "success");

    expect(store.getState().workspaceContextRead).toMatchObject({
      retryVersion: 1,
      errors: { repositories: null },
      retryAfterMs: { repositories: null },
    });
  });

  it("does not let an abandoned request settle a replacement collection read", () => {
    const store = makeStore();
    const setRead = store.getState().setWorkspaceContextRead;

    Reflect.apply(setRead, undefined, [
      "workflows",
      ARCHIVED_WORKSPACE_ID,
      0,
      "pending",
      undefined,
      "request-a",
    ]);
    Reflect.apply(setRead, undefined, [
      "workflows",
      ARCHIVED_WORKSPACE_ID,
      0,
      "pending",
      undefined,
      "request-b",
    ]);
    Reflect.apply(setRead, undefined, [
      "workflows",
      ARCHIVED_WORKSPACE_ID,
      0,
      "cancelled",
      undefined,
      "request-a",
    ]);

    expect(store.getState().workspaceContextRead.pending.workflows).toBe(true);
    expect(store.getState().workspaceContextRead.requestIds.workflows).toBe("request-b");

    Reflect.apply(setRead, undefined, [
      "workflows",
      ARCHIVED_WORKSPACE_ID,
      0,
      "success",
      undefined,
      "request-b",
    ]);
    expect(store.getState().workspaceContextRead.pending.workflows).toBe(false);
  });

  it("tracks snapshot failures independently from the workflow list request", () => {
    const store = makeStore();

    store
      .getState()
      .setWorkspaceSnapshotRead(ARCHIVED_WORKSPACE_ID, 0, "pending", undefined, "snapshot-a");
    store
      .getState()
      .setWorkspaceSnapshotRead(ARCHIVED_WORKSPACE_ID, 0, "transient", 4_000, "snapshot-a");

    expect(store.getState().workspaceContextRead).toMatchObject({
      snapshotPending: false,
      snapshotError: "transient",
      snapshotRetryAfterMs: 4_000,
      snapshotRequestId: null,
    });

    store
      .getState()
      .setWorkspaceSnapshotRead(ARCHIVED_WORKSPACE_ID, 0, "pending", undefined, "snapshot-b");
    store
      .getState()
      .setWorkspaceSnapshotRead(ARCHIVED_WORKSPACE_ID, 0, "cancelled", undefined, "snapshot-a");
    expect(store.getState().workspaceContextRead.snapshotPending).toBe(true);
    expect(store.getState().workspaceContextRead.snapshotRequestId).toBe("snapshot-b");
  });
});

describe("kanban slice archived sidebar projection", () => {
  it("stores archived tasks separately from active kanban state and deduplicates IDs", () => {
    const store = makeStore();
    const task = {
      id: TASK_ID,
      workflowId: WORKFLOW_ID,
      workflowStepId: "step-a",
      title: "Archived",
      position: 0,
    };

    store.getState().setSidebarArchivedTasks(ARCHIVED_WORKSPACE_ID, [task]);
    store
      .getState()
      .upsertSidebarArchivedTask(ARCHIVED_WORKSPACE_ID, { ...task, title: "Updated" });

    expect(store.getState().sidebarArchivedTasks.itemsByWorkspaceId[ARCHIVED_WORKSPACE_ID]).toEqual(
      [{ ...task, title: "Updated" }],
    );
    expect(store.getState().kanban.tasks).toEqual([]);
    expect(store.getState().sidebarArchivedTasks.loadedByWorkspaceId[ARCHIVED_WORKSPACE_ID]).toBe(
      true,
    );
  });

  it("creates a workspace bucket when upserting its first archived task", () => {
    const store = makeStore();
    const task = {
      id: TASK_ID,
      workflowId: WORKFLOW_ID,
      workflowStepId: "step-a",
      title: "Archived",
      position: 0,
    };

    store.getState().upsertSidebarArchivedTask("workspace-new", task);

    expect(store.getState().sidebarArchivedTasks.itemsByWorkspaceId["workspace-new"]).toEqual([
      task,
    ]);
  });

  it("does not let a stale load replace archive, unarchive, or delete events", () => {
    const store = makeStore();
    type ArchivedStateWithRevision = KanbanSlice["sidebarArchivedTasks"] & {
      revisionByWorkspaceId?: Record<string, number>;
    };
    const revision = () =>
      (store.getState().sidebarArchivedTasks as ArchivedStateWithRevision).revisionByWorkspaceId?.[
        ARCHIVED_WORKSPACE_ID
      ] ?? 0;
    const setArchivedTasks = store.getState().setSidebarArchivedTasks as unknown as (
      workspaceId: string,
      tasks: KanbanSlice["sidebarArchivedTasks"]["itemsByWorkspaceId"][string],
      expectedRevision?: number,
    ) => boolean;
    const task = {
      id: TASK_ID,
      workflowId: WORKFLOW_ID,
      workflowStepId: "step-a",
      title: "Loaded",
      position: 0,
    };

    setArchivedTasks(ARCHIVED_WORKSPACE_ID, [task]);
    const beforeArchive = revision();
    store
      .getState()
      .upsertSidebarArchivedTask(ARCHIVED_WORKSPACE_ID, { ...task, title: "Archived live" });
    expect(setArchivedTasks(ARCHIVED_WORKSPACE_ID, [task], beforeArchive)).toBe(false);
    expect(store.getState().sidebarArchivedTasks.itemsByWorkspaceId[ARCHIVED_WORKSPACE_ID]).toEqual(
      [{ ...task, title: "Archived live" }],
    );

    const beforeUnarchive = revision();
    store.getState().removeSidebarArchivedTask(TASK_ID, ARCHIVED_WORKSPACE_ID);
    expect(setArchivedTasks(ARCHIVED_WORKSPACE_ID, [task], beforeUnarchive)).toBe(false);
    expect(store.getState().sidebarArchivedTasks.itemsByWorkspaceId[ARCHIVED_WORKSPACE_ID]).toEqual(
      [],
    );

    setArchivedTasks(ARCHIVED_WORKSPACE_ID, [task]);
    const beforeDelete = revision();
    store.getState().removeSidebarArchivedTask(TASK_ID, ARCHIVED_WORKSPACE_ID);
    expect(setArchivedTasks(ARCHIVED_WORKSPACE_ID, [task], beforeDelete)).toBe(false);
    expect(store.getState().sidebarArchivedTasks.itemsByWorkspaceId[ARCHIVED_WORKSPACE_ID]).toEqual(
      [],
    );
  });
});

describe("kanban slice resume-skipped marker", () => {
  it("records the skip for any session and clears it on request", () => {
    const store = makeStore();
    store.getState().setResumeSkipped("session-idle", true);
    expect(store.getState().tasks.resumeSkippedSessionIds["session-idle"]).toBe(true);

    store.getState().setResumeSkipped("session-idle", false);
    expect(store.getState().tasks.resumeSkippedSessionIds["session-idle"]).toBeUndefined();
  });

  it("records the skip as a plain keyed write (the STARTING/RUNNING guard lives at the hook call site)", () => {
    // The slice is deliberately a dumb write: the guard that refuses to mark
    // a starting/running session resume-skipped reads the live session row
    // with typed store access in use-session-resumption's skip branch. A
    // direct slice call records unconditionally.
    const store = makeStore();
    store.getState().setResumeSkipped("session-running", true);
    expect(store.getState().tasks.resumeSkippedSessionIds["session-running"]).toBe(true);

    store.getState().setResumeSkipped("session-running", false);
    expect(store.getState().tasks.resumeSkippedSessionIds["session-running"]).toBeUndefined();
  });
});

describe("kanban slice step order revision and in-flight reorder tracking", () => {
  it("records and overwrites a step's last-applied order revision", () => {
    const store = makeStore();
    store.getState().setStepOrderRevision("step-1", 3);
    expect(store.getState().kanbanMulti.orderRevisionByStepId["step-1"]).toBe(3);

    store.getState().setStepOrderRevision("step-1", 4);
    expect(store.getState().kanbanMulti.orderRevisionByStepId["step-1"]).toBe(4);
  });

  it("marks and clears a band as pending, keyed by step and band", () => {
    const store = makeStore();
    store.getState().setBandReorderPending("step-1", "admitted", true);
    expect(store.getState().kanbanMulti.pendingReorderBandKeys["step-1:admitted"]).toBe(true);
    expect(store.getState().kanbanMulti.pendingReorderBandKeys["step-1:queued"]).toBeUndefined();

    store.getState().setBandReorderPending("step-1", "admitted", false);
    expect(store.getState().kanbanMulti.pendingReorderBandKeys["step-1:admitted"]).toBeUndefined();
  });
});
