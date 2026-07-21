import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, WorkflowPayload } from "@/lib/types/backend";
import { registerWorkflowsHandlers } from "./workflows";

type WorkflowItem = { id: string; workspaceId: string; name: string; hidden?: boolean };

const STEP_COLOR = "bg-blue-500";
const TIMESTAMP = "2026-01-01T00:00:00Z";

function makeStore(items: WorkflowItem[], activeId: string | null) {
  let state = {
    workflows: { items, activeId },
    workspaces: { activeId: "ws-1" },
    kanban: { workflowId: null, steps: [], tasks: [] },
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

function updatedMessage(payload: WorkflowPayload): BackendMessageMap["workflow.updated"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "workflow.updated",
    payload,
    timestamp: TIMESTAMP,
  };
}

function createdMessage(payload: WorkflowPayload): BackendMessageMap["workflow.created"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "workflow.created",
    payload,
    timestamp: TIMESTAMP,
  };
}

function stepUpdatedMessage(
  step: BackendMessageMap["workflow.step.updated"]["payload"]["step"],
): BackendMessageMap["workflow.step.updated"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "workflow.step.updated",
    payload: { step },
    timestamp: TIMESTAMP,
  };
}

function stepDeletedMessage(
  step: BackendMessageMap["workflow.step.deleted"]["payload"]["step"],
): BackendMessageMap["workflow.step.deleted"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "workflow.step.deleted",
    payload: { step },
    timestamp: TIMESTAMP,
  };
}

describe("workflow.created handler — preserves user filter", () => {
  it("does not promote a new workflow when activeId is null ('All Workflows')", () => {
    const store = makeStore(
      [{ id: "wf-1", workspaceId: "ws-1", name: "Existing", hidden: false }],
      null,
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.created"]?.(
      createdMessage({ id: "wf-2", workspace_id: "ws-1", name: "Brand New" }),
    );

    expect(store.getState().workflows.activeId).toBeNull();
    expect(store.getState().workflows.items.map((i) => i.id)).toEqual(["wf-2", "wf-1"]);
  });

  it("leaves an existing activeId untouched when a new workflow appears", () => {
    const store = makeStore(
      [{ id: "wf-1", workspaceId: "ws-1", name: "Existing", hidden: false }],
      "wf-1",
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.created"]?.(
      createdMessage({ id: "wf-2", workspace_id: "ws-1", name: "Brand New" }),
    );

    expect(store.getState().workflows.activeId).toBe("wf-1");
  });
});

describe("workflow.updated handler — hidden flag reconciles activeId", () => {
  it("clears activeId to next visible workflow when active becomes hidden", () => {
    const store = makeStore(
      [
        { id: "wf-1", workspaceId: "ws-1", name: "Improve Kandev", hidden: false },
        { id: "wf-2", workspaceId: "ws-1", name: "Default", hidden: false },
      ],
      "wf-1",
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.updated"]?.(
      updatedMessage({ id: "wf-1", workspace_id: "ws-1", name: "Improve Kandev", hidden: true }),
    );

    expect(store.getState().workflows.activeId).toBe("wf-2");
    expect(store.getState().workflows.items.find((i) => i.id === "wf-1")?.hidden).toBe(true);
  });

  it("clears activeId to null when no visible workflow remains", () => {
    const store = makeStore(
      [{ id: "wf-1", workspaceId: "ws-1", name: "Only One", hidden: false }],
      "wf-1",
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.updated"]?.(
      updatedMessage({ id: "wf-1", workspace_id: "ws-1", name: "Only One", hidden: true }),
    );

    expect(store.getState().workflows.activeId).toBeNull();
  });

  it("leaves activeId untouched when a non-active workflow becomes hidden", () => {
    const store = makeStore(
      [
        { id: "wf-1", workspaceId: "ws-1", name: "Active", hidden: false },
        { id: "wf-2", workspaceId: "ws-1", name: "Other", hidden: false },
      ],
      "wf-1",
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.updated"]?.(
      updatedMessage({ id: "wf-2", workspace_id: "ws-1", name: "Other", hidden: true }),
    );

    expect(store.getState().workflows.activeId).toBe("wf-1");
  });

  it("leaves activeId untouched when payload omits hidden", () => {
    const store = makeStore(
      [{ id: "wf-1", workspaceId: "ws-1", name: "Old Name", hidden: false }],
      "wf-1",
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.updated"]?.(
      updatedMessage({ id: "wf-1", workspace_id: "ws-1", name: "New Name" }),
    );

    expect(store.getState().workflows.activeId).toBe("wf-1");
    expect(store.getState().workflows.items[0]?.name).toBe("New Name");
  });
});

describe("workflow step handlers", () => {
  it("preserves WIP fields from step update payloads", () => {
    const store = makeStore([{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }], "wf-1");
    store.setState({
      ...store.getState(),
      kanban: {
        workflowId: "wf-1",
        steps: [{ id: "step-1", title: "Review", color: STEP_COLOR, position: 1 }],
        tasks: [],
      },
    } as AppState);
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.step.updated"]?.(
      stepUpdatedMessage({
        id: "step-1",
        workflow_id: "wf-1",
        name: "Review",
        state: "",
        position: 1,
        color: STEP_COLOR,
        wip_limit: 2,
        pull_from_step_id: "step-0",
      }),
    );

    expect(store.getState().kanban.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("re-sorts cached task session lists when a step position changes", () => {
    const store = makeStore([{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }], "wf-1");
    store.setState({
      ...store.getState(),
      kanban: {
        workflowId: "wf-1",
        steps: [
          { id: "step-spec", title: "Spec", color: STEP_COLOR, position: 0 },
          { id: "step-work", title: "Work", color: STEP_COLOR, position: 1 },
        ],
        tasks: [],
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "task-1": [
            { id: "s-work", workflow_step_id: "step-work", started_at: "2025-06-01T00:00:00Z" },
            { id: "s-spec", workflow_step_id: "step-spec", started_at: "2025-06-01T00:00:01Z" },
          ],
        },
      },
    } as unknown as AppState);
    const handlers = registerWorkflowsHandlers(store);

    // Flip "Work" to sort before "Spec" — the cached session list for
    // task-1 was ordered for the old positions and must be re-derived.
    handlers["workflow.step.updated"]?.(
      stepUpdatedMessage({
        id: "step-work",
        workflow_id: "wf-1",
        name: "Work",
        state: "",
        position: -1,
        color: STEP_COLOR,
      }),
    );

    expect(store.getState().taskSessionsByTask.itemsByTaskId["task-1"].map((s) => s.id)).toEqual([
      "s-work",
      "s-spec",
    ]);
  });
});

describe("workflow.step.deleted handler", () => {
  it("re-sorts cached task session lists when a step is deleted", () => {
    const store = makeStore([{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }], "wf-1");
    store.setState({
      ...store.getState(),
      kanban: {
        workflowId: "wf-1",
        steps: [
          { id: "step-spec", title: "Spec", color: STEP_COLOR, position: 0 },
          { id: "step-review", title: "Review", color: STEP_COLOR, position: 1 },
          { id: "step-work", title: "Work", color: STEP_COLOR, position: 2 },
        ],
        tasks: [],
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "task-1": [
            { id: "s-work", workflow_step_id: "step-work", started_at: "2025-06-01T00:00:00Z" },
            { id: "s-spec", workflow_step_id: "step-spec", started_at: "2025-06-01T00:00:01Z" },
          ],
        },
      },
    } as unknown as AppState);
    const handlers = registerWorkflowsHandlers(store);

    // Deleting the middle step shifts "Work"'s effective position — the
    // cached session list for task-1 must be re-derived, not left stale.
    handlers["workflow.step.deleted"]?.(
      stepDeletedMessage({
        id: "step-review",
        workflow_id: "wf-1",
        name: "Review",
        state: "",
        position: 1,
        color: STEP_COLOR,
      }),
    );

    expect(store.getState().kanban.steps.map((s) => s.id)).toEqual(["step-spec", "step-work"]);
    expect(store.getState().taskSessionsByTask.itemsByTaskId["task-1"].map((s) => s.id)).toEqual([
      "s-spec",
      "s-work",
    ]);
  });
});
