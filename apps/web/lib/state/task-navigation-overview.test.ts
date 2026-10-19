import { afterEach, expect, it, vi } from "vitest";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { applyTaskOverviewEvent } from "@/lib/ws/handlers/task-overview";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import { createAppStore } from "./store";
import { defaultState } from "./default-state";
import { beginTaskNavigation, readTaskNavigationIdentity } from "./task-navigation-reads";

const TARGET = "task-target";
const OTHER = "task-other";
const WORKSPACE = "workspace";
const WORKFLOW = "workflow";
const STAMP = "2026-10-15T10:00:00Z";

function task(id = TARGET, step = "working"): Task {
  return {
    id: taskId(id),
    title: "Task",
    description: "",
    state: "CREATED",
    priority: "medium",
    workspace_id: workspaceId(WORKSPACE),
    workflow_id: workflowId(WORKFLOW),
    workflow_step_id: step,
    updated_at: STAMP,
    created_at: STAMP,
    repositories: [],
    position: 0,
  };
}

function setup(activeTaskId: string | null) {
  const store = createAppStore({
    workspaces: { ...defaultState.workspaces, activeId: WORKSPACE, items: [] },
    tasks: { ...defaultState.tasks, activeTaskId },
    kanban: {
      ...defaultState.kanban,
      workflowId: WORKFLOW,
      tasks: [toKanbanTask(task()), toKanbanTask(task(OTHER))],
      steps: [],
    },
  });
  let resolve!: (response: Response) => void;
  const response = new Promise<Response>((yes) => {
    resolve = yes;
  });
  const fetchMock = vi.fn((url: string) =>
    String(url).endsWith("/sessions")
      ? Promise.resolve(Response.json({ sessions: [], total: 0 }))
      : response,
  );
  vi.stubGlobal("fetch", fetchMock);
  return { store, resolve, fetchMock };
}

afterEach(() => vi.unstubAllGlobals());

it.each([null, OTHER])(
  "publishes an admitted navigation before activation and preserves live state on cached reuse (%s)",
  async (activeTaskId) => {
    const { store, resolve, fetchMock } = setup(activeTaskId);
    const context = beginTaskNavigation(store, {}, `${TARGET}\u0000`);
    const pending = readTaskNavigationIdentity(store, TARGET, { context });
    expect(store.getState().tasks.activeTaskId).toBe(activeTaskId);
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith(`/tasks/${TARGET}`)),
    ).toHaveLength(1);
    resolve(Response.json(task(TARGET, "review")));
    await pending;
    expect(store.getState().taskOverview.byId[TARGET].workflowStepId).toBe("review");
    expect(store.getState().tasks.activeTaskId).toBe(activeTaskId);
    expect(store.getState().taskOverview.reads).toEqual({});

    store.setState((state) => applyTaskOverviewEvent(state, task(TARGET, "done")));
    await readTaskNavigationIdentity(store, TARGET, { context });
    expect(store.getState().taskOverview.byId[TARGET].workflowStepId).toBe("done");
    expect(store.getState().taskOverview.reads).toEqual({});
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith(`/tasks/${TARGET}`)),
    ).toHaveLength(1);
    store.getState().setActiveTask(TARGET);
    expect(store.getState().taskOverview.byId[TARGET].workflowStepId).toBe("done");
  },
);

it("discards a previously admitted navigation before it can publish", async () => {
  const { store, resolve } = setup(OTHER);
  const owner = {};
  const context = beginTaskNavigation(store, owner, `${TARGET}\u0000`);
  const pending = readTaskNavigationIdentity(store, TARGET, { context });
  const outcome = pending.catch((error: unknown) => error);
  beginTaskNavigation(store, owner, `${OTHER}\u0000`);
  resolve(Response.json(task(TARGET, "review")));
  await expect(outcome).resolves.toMatchObject({ name: "AbortError" });
  await Promise.resolve();
  expect(store.getState().taskOverview.byId[TARGET].workflowStepId).toBe("working");
  expect(store.getState().tasks.activeTaskId).toBe(OTHER);
  await vi.waitFor(() => expect(store.getState().taskOverview.reads).toEqual({}));
});
