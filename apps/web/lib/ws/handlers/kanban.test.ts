import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerKanbanHandlers } from "./kanban";

const WORKFLOW_ID = "wf1";
const TASK_ID = "t1";
const STEP_ID = "s1";
const TASK_TITLE = "T1";
const UPDATED_TITLE = "T1 updated";
const REPO_A_ID = "repo-a";
const REPO_B_ID = "repo-b";
const REPO_LINK_A_ID = "link-a";
const FEATURE_BRANCH = "feature/x";

function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: null, steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    ...initial,
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      state =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
    },
    subscribe: () => () => {},
    destroy: () => {},
    getInitialState: () => state,
  } as unknown as StoreApi<AppState>;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function makeUpdateMessage(workflowId: string, tasks: unknown[], steps: unknown[] = []): any {
  return {
    id: "msg-1",
    type: "notification",
    action: "kanban.update",
    payload: { workflowId, tasks, steps },
  };
}

describe("kanban.update handler — primarySessionId preservation", () => {
  it("preserves workflow step WIP fields", () => {
    const store = makeStore();
    const handler = registerKanbanHandlers(store)["kanban.update"]!;

    handler(
      makeUpdateMessage(
        WORKFLOW_ID,
        [],
        [
          {
            id: STEP_ID,
            title: "Review",
            position: 1,
            color: "bg-blue-500",
            wip_limit: 2,
            pull_from_step_id: "step-0",
          },
        ],
      ),
    );

    expect(store.getState().kanban.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("preserves primarySessionId from existing tasks", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            primarySessionId: "sess-primary",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        {
          id: TASK_ID,
          workflowStepId: STEP_ID,
          title: UPDATED_TITLE,
          position: 0,
          state: "IN_PROGRESS",
        },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.primarySessionId).toBe("sess-primary");
    expect(task?.title).toBe(UPDATED_TITLE);
  });

  it("preserves primarySessionState from existing tasks", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            primarySessionId: "sess-primary",
            primarySessionState: "RUNNING",
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: TASK_ID, workflowStepId: STEP_ID, title: TASK_TITLE, position: 0 },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.primarySessionState).toBe("RUNNING");
  });

  it("new tasks start with undefined primarySessionId", () => {
    const store = makeStore({
      kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [] },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        { id: "t-new", workflowStepId: STEP_ID, title: "New Task", position: 0 },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === "t-new");
    expect(task).toBeDefined();
    expect(task?.primarySessionId).toBeUndefined();
  });
});

describe("kanban.update handler — repository preservation", () => {
  it("preserves existing repositories when kanban.update omits repo metadata", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            repositoryId: REPO_A_ID,
            repositories: [
              {
                id: REPO_LINK_A_ID,
                repository_id: REPO_A_ID,
                base_branch: "main",
                checkout_branch: FEATURE_BRANCH,
                position: 0,
              },
            ],
          },
        ],
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        {
          id: TASK_ID,
          workflowStepId: STEP_ID,
          title: UPDATED_TITLE,
          position: 0,
          state: "CREATED",
        },
      ]),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.repositoryId).toBe(REPO_A_ID);
    expect(task?.repositories).toEqual([
      {
        id: REPO_LINK_A_ID,
        repository_id: REPO_A_ID,
        base_branch: "main",
        checkout_branch: FEATURE_BRANCH,
        position: 0,
      },
    ]);
  });
});

describe("kanban.update handler — repository switch", () => {
  it("does not restore stale snapshot repositories when repository_id changes", () => {
    const repoA = [
      {
        id: REPO_LINK_A_ID,
        repository_id: REPO_A_ID,
        base_branch: "main",
        checkout_branch: FEATURE_BRANCH,
        position: 0,
      },
    ];
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: TASK_TITLE,
            position: 0,
            repositoryId: REPO_A_ID,
            repositories: repoA,
          },
        ],
      },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          [WORKFLOW_ID]: {
            workflowId: WORKFLOW_ID,
            workflowName: "WF1",
            steps: [],
            tasks: [
              {
                id: TASK_ID,
                workflowStepId: STEP_ID,
                title: TASK_TITLE,
                position: 0,
                repositoryId: REPO_A_ID,
                repositories: repoA,
              },
            ],
          },
        },
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage(WORKFLOW_ID, [
        {
          id: TASK_ID,
          workflowStepId: STEP_ID,
          title: UPDATED_TITLE,
          position: 0,
          repository_id: REPO_B_ID,
        },
      ]),
    );

    const snapshotTask = store
      .getState()
      .kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === TASK_ID);
    expect(snapshotTask?.repositoryId).toBe(REPO_B_ID);
    expect(snapshotTask?.repositories).toBeUndefined();
  });
});

describe("kanban.update handler — explicit-null primary preservation", () => {
  it("does not restore stale snapshot value when primarySessionId is explicitly cleared", () => {
    // Repro for the multi-snapshot null-preservation bug: when task.updated
    // clears primarySessionId to null in kanban.tasks, the multi-snapshot must
    // accept the null rather than fall back to a stale value.
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [
          {
            id: "t1",
            workflowStepId: "s1",
            title: "T1",
            position: 0,
            primarySessionId: null,
          },
        ],
      },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: {
            workflowId: "wf1",
            workflowName: "WF1",
            steps: [],
            tasks: [
              {
                id: "t1",
                workflowStepId: "s1",
                title: "T1",
                position: 0,
                primarySessionId: "stale-session",
              },
            ],
          },
        },
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0 }]),
    );

    const snapshot = store.getState().kanbanMulti.snapshots["wf1"];
    const task = snapshot?.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBeNull();
  });
});

describe("kanban.update handler — multi-snapshot primary lookup", () => {
  it("preserves primarySessionId in kanbanMulti snapshot", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [] },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: {
            workflowId: "wf1",
            workflowName: "WF1",
            steps: [],
            tasks: [
              {
                id: "t1",
                workflowStepId: "s1",
                title: "T1",
                position: 0,
                primarySessionId: "sess-multi-primary",
              },
            ],
          },
        },
      },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [{ id: "t1", workflowStepId: "s1", title: "T1", position: 0 }]),
    );

    const snapshot = store.getState().kanbanMulti.snapshots["wf1"];
    const task = snapshot?.tasks.find((t) => t.id === "t1");
    expect(task?.primarySessionId).toBe("sess-multi-primary");
  });
});

describe("kanban.update handler — task filtering", () => {
  it("skips ephemeral tasks", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [] },
    } as Partial<AppState>);

    const handler = registerKanbanHandlers(store)["kanban.update"]!;
    handler(
      makeUpdateMessage("wf1", [
        { id: "t1", workflowStepId: "s1", title: "Real", position: 0 },
        {
          id: "t-ephemeral",
          workflowStepId: "s1",
          title: "Ephemeral",
          position: 1,
          is_ephemeral: true,
        },
      ]),
    );

    expect(store.getState().kanban.tasks).toHaveLength(1);
    expect(store.getState().kanban.tasks[0].id).toBe("t1");
  });
});
