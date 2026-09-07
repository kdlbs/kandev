import { act, cleanup, render } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";

const shellRenderCounts = vi.hoisted(() => new Map<string, number>());

vi.mock("@/components/kanban-card-content", () => ({
  KanbanCardShell: ({ task }: { task: { id: string } }) => {
    shellRenderCounts.set(task.id, (shellRenderCounts.get(task.id) ?? 0) + 1);
    return <div data-testid={`card-shell-${task.id}`} />;
  },
}));

import { KanbanCard } from "./kanban-card";
import { StateProvider, useAppStoreApi } from "./state-provider";
import { ToastProvider } from "./toast-provider";
import { defaultState } from "@/lib/state/default-state";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

const WORKSPACE_ID = "workspace-1";
const WORKFLOW_A = "workflow-a";
const WORKFLOW_B = "workflow-b";
const STEP_A = { id: "step-a", title: "Todo", color: "bg-blue-500", position: 0 };
const STEP_B = { id: "step-b", title: "Todo", color: "bg-blue-500", position: 0 };
const TASK_A: KanbanState["tasks"][number] = {
  id: "task-a",
  workflowId: WORKFLOW_A,
  title: "Task A",
  workflowStepId: STEP_A.id,
  position: 0,
};
const TASK_B: KanbanState["tasks"][number] = {
  id: "task-b",
  workflowId: WORKFLOW_B,
  title: "Task B",
  workflowStepId: STEP_B.id,
  position: 0,
};

function StoreCapture({ holder }: { holder: { current: StoreApi<AppState> | null } }) {
  holder.current = useAppStoreApi();
  return null;
}

beforeEach(() => shellRenderCounts.clear());
afterEach(cleanup);

describe("KanbanCard render isolation", () => {
  it("does not rerender when a task in another workflow updates", () => {
    const holder: { current: StoreApi<AppState> | null } = { current: null };
    render(
      <ToastProvider>
        <StateProvider
          initialState={{
            workspaces: { ...defaultState.workspaces, activeId: WORKSPACE_ID },
            workflows: {
              activeId: null,
              items: [
                { id: WORKFLOW_A, workspaceId: WORKSPACE_ID, name: "Workflow A" },
                { id: WORKFLOW_B, workspaceId: WORKSPACE_ID, name: "Workflow B" },
              ],
            },
            kanbanMulti: {
              isLoading: false,
              snapshots: {
                [WORKFLOW_A]: {
                  workflowId: WORKFLOW_A,
                  workflowName: "Workflow A",
                  steps: [STEP_A],
                  tasks: [TASK_A],
                },
                [WORKFLOW_B]: {
                  workflowId: WORKFLOW_B,
                  workflowName: "Workflow B",
                  steps: [STEP_B],
                  tasks: [TASK_B],
                },
              },
            },
          }}
        >
          <StoreCapture holder={holder} />
          <KanbanCard
            task={TASK_A}
            workspaceId={WORKSPACE_ID}
            steps={[STEP_A]}
            externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
          />
        </StateProvider>
      </ToastProvider>,
    );

    const store = holder.current;
    if (!store) throw new Error("Store was not captured");
    act(() => {
      store.getState().updateMultiTask(WORKFLOW_B, { ...TASK_B, title: "Updated Task B" });
    });

    expect(shellRenderCounts.get(TASK_A.id)).toBe(1);
  });
});
