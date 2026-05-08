import { describe, it, expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, WorkflowPayload } from "@/lib/types/backend";
import { registerWorkflowsHandlers } from "./workflows";

type WorkflowItem = { id: string; workspaceId: string; name: string; hidden?: boolean };

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
    timestamp: "2026-01-01T00:00:00Z",
  };
}

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
