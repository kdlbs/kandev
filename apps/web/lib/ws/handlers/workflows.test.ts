import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, WorkflowPayload } from "@/lib/types/backend";
import { registerWorkflowsHandlers } from "./workflows";

type WorkflowItem = { id: string; workspaceId: string; name: string; hidden?: boolean };

const MESSAGE_TIMESTAMP = "2026-01-01T00:00:00Z";
const REVIEW_STEP_ID = "step-1";
const REVIEW_STEP_NAME = "Review";
const REVIEW_STEP_COLOR = "bg-blue-500";

function makeStore(items: WorkflowItem[], activeId: string | null) {
  let state = {
    workflows: { items, activeId },
    workspaces: { activeId: "ws-1" },
    kanban: { workflowId: null, steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
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
    timestamp: MESSAGE_TIMESTAMP,
  };
}

function createdMessage(payload: WorkflowPayload): BackendMessageMap["workflow.created"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "workflow.created",
    payload,
    timestamp: MESSAGE_TIMESTAMP,
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
    timestamp: MESSAGE_TIMESTAMP,
  };
}

function stepCreatedMessage(
  step: BackendMessageMap["workflow.step.created"]["payload"]["step"],
): BackendMessageMap["workflow.step.created"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "workflow.step.created",
    payload: { step },
    timestamp: MESSAGE_TIMESTAMP,
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
    timestamp: MESSAGE_TIMESTAMP,
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

  it("propagates description and prompt from the payload", () => {
    const store = makeStore(
      [{ id: "wf-1", workspaceId: "ws-1", name: "Workflow", hidden: false }],
      "wf-1",
    );
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.updated"]?.(
      updatedMessage({
        id: "wf-1",
        workspace_id: "ws-1",
        name: "Workflow",
        description: "Updated from another tab",
        prompt: "Updated prompt",
      }),
    );

    const item = store.getState().workflows.items[0];
    expect(item?.description).toBe("Updated from another tab");
    expect(item?.prompt).toBe("Updated prompt");
  });
});

describe("workflow step handlers", () => {
  it("preserves WIP fields from step update payloads", () => {
    const store = makeStore([{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }], "wf-1");
    store.setState({
      ...store.getState(),
      kanban: {
        workflowId: "wf-1",
        steps: [
          { id: REVIEW_STEP_ID, title: REVIEW_STEP_NAME, color: REVIEW_STEP_COLOR, position: 1 },
        ],
        tasks: [],
      },
    } as AppState);
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.step.updated"]?.(
      stepUpdatedMessage({
        id: REVIEW_STEP_ID,
        workflow_id: "wf-1",
        name: REVIEW_STEP_NAME,
        state: "",
        position: 1,
        color: REVIEW_STEP_COLOR,
        wip_limit: 2,
        pull_from_step_id: "step-0",
      }),
    );

    expect(store.getState().kanban.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("keeps cached workflow snapshots in sync for step changes", () => {
    const store = makeStore([{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }], null);
    const initialStep = {
      id: REVIEW_STEP_ID,
      title: REVIEW_STEP_NAME,
      color: REVIEW_STEP_COLOR,
      position: 1,
      prompt: "Review the task",
    };
    store.setState({
      ...store.getState(),
      kanbanMulti: {
        snapshots: {
          "wf-1": {
            workflowId: "wf-1",
            workflowName: "Workflow",
            steps: [initialStep],
            tasks: [],
          },
        },
        isLoading: false,
      },
    } as AppState);
    const handlers = registerWorkflowsHandlers(store);

    handlers["workflow.step.created"]?.(
      stepCreatedMessage({
        id: "step-2",
        workflow_id: "wf-1",
        name: "Done",
        state: "done",
        position: 2,
        color: "bg-green-500",
      }),
    );
    handlers["workflow.step.updated"]?.(
      stepUpdatedMessage({
        id: REVIEW_STEP_ID,
        workflow_id: "wf-1",
        name: REVIEW_STEP_NAME,
        state: "review",
        position: 1,
        color: REVIEW_STEP_COLOR,
        prompt: "Review the task carefully",
      }),
    );
    handlers["workflow.step.deleted"]?.(
      stepDeletedMessage({
        id: "step-2",
        workflow_id: "wf-1",
        name: "Done",
        state: "done",
        position: 2,
        color: "bg-green-500",
      }),
    );

    const snapshotSteps = store.getState().kanbanMulti.snapshots["wf-1"]?.steps;
    expect(snapshotSteps?.map((step) => step.id)).toEqual([REVIEW_STEP_ID]);
    expect(snapshotSteps?.[0]).toMatchObject({
      title: REVIEW_STEP_NAME,
      position: 1,
      prompt: "Review the task carefully",
    });
  });
});
