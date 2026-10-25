import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { expect, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { useWorkflowSnapshotById } from "./use-all-workflow-snapshots";
import { useCrossStepMove } from "./use-swimlane-kanban-dnd";

const WORKFLOW_ID = "workflow-a";
const ORIGINAL_SESSION = "session-original";
const STALE_STEP = "stale-stage";
const STALE_UPDATED_AT = "2026-10-22T09:00:00Z";
const wireTask = {
  task_id: "task-a",
  workflow_id: WORKFLOW_ID,
  workflow_step_id: "source",
  // i18n-exempt: Fixed test fixture title, not product copy.
  title: "Original",
  description: "",
  state: "CREATED" as const,
  is_ephemeral: false,
  position: 7,
  primary_session_id: ORIGINAL_SESSION,
  created_at: "2026-10-22T10:00:00Z",
  updated_at: "2026-10-22T10:00:00Z",
};
const original = toKanbanTask(wireTask);
// i18n-exempt: Fixed test fixture title, not product copy.
const sibling = { ...original, id: "task-b", title: "Sibling" };
const snapshot = {
  workflowId: WORKFLOW_ID,
  workflowName: "Workflow",
  steps: [],
  tasks: [original, sibling],
};
function Wrapper({
  children,
  workspaceId = null,
}: {
  children: ReactNode;
  workspaceId?: string | null;
}) {
  return (
    <StateProvider
      initialState={{
        workspaces: { activeId: workspaceId, items: [] },
        kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [original, sibling] },
        kanbanMulti: {
          snapshots: { [WORKFLOW_ID]: snapshot },
          isLoading: false,
          orderRevisionByStepId: {},
          pendingReorderBandKeys: {},
          withheldReorderByBandKey: {},
        },
      }}
    >
      <ToastProvider>{children}</ToastProvider>
    </StateProvider>
  );
}
function SnapshotWrapper({ children }: { children: ReactNode }) {
  return <Wrapper workspaceId="workspace-a">{children}</Wrapper>;
}
function current(store: StoreApi<AppState>, taskId = "task-a") {
  return store.getState().kanbanMulti.snapshots[WORKFLOW_ID]?.tasks.find((t) => t.id === taskId);
}
function update(store: StoreApi<AppState>, patch: Record<string, unknown>) {
  act(() =>
    registerTasksHandlers(store)["task.updated"]!({
      id: "authoritative",
      type: "notification",
      action: "task.updated",
      payload: { ...wireTask, ...patch },
    }),
  );
}
function setup(withSnapshot = false) {
  const snapshotResponses: Array<(response: Response) => void> = [];
  const responses: Array<(response: Response) => void> = [];
  const requests: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      if (new URL(value, "http://localhost").pathname.endsWith("/snapshot")) {
        expect(init?.method ?? "GET").toBe("GET");
        return new Promise<Response>((resolve) => snapshotResponses.push(resolve));
      }
      expect(new URL(value, "http://localhost").pathname).toMatch(
        /\/api\/v1\/tasks\/task-[ab]\/move$/,
      );
      expect(init?.method).toBe("POST");
      requests.push(JSON.parse(init?.body as string));
      return new Promise<Response>((resolve) => responses.push(resolve));
    }),
  );
  const onMoveError = vi.fn();
  const hook = renderHook(
    (props: { workflowId: string; onError: typeof onMoveError; snapshotEnabled?: boolean }) => {
      useWorkflowSnapshotById(
        props.snapshotEnabled ? "workspace-a" : null,
        props.snapshotEnabled ? props.workflowId : null,
      );
      return { store: useAppStoreApi(), move: useCrossStepMove(props.workflowId, props.onError) };
    },
    {
      wrapper: withSnapshot ? SnapshotWrapper : Wrapper,
      initialProps: { workflowId: WORKFLOW_ID, onError: onMoveError },
    },
  );
  function start(task = original, target = "optimistic") {
    let pending!: Promise<void>;
    act(() => {
      pending = hook.result.current.move(task.id, target, task);
    });
    return pending;
  }
  async function settle(pending: Promise<void>, index = 0, success = false) {
    await act(async () => {
      responses[index](
        new Response(JSON.stringify(success ? {} : { error: "Original move rejected" }), {
          status: success ? 200 : 500,
        }),
      );
      await pending;
    });
  }
  return {
    ...hook,
    start,
    settle,
    requests,
    responses,
    snapshotResponses,
    onMoveError,
    store: hook.result.current.store,
  };
}

export {
  setup,
  current,
  update,
  WORKFLOW_ID,
  ORIGINAL_SESSION,
  STALE_STEP,
  STALE_UPDATED_AT,
  original,
  sibling,
  wireTask,
};
