import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId, sessionId } from "@/lib/types/ids";
import type { TaskNavigationReadSnapshot } from "@/lib/state/task-navigation-reads";
import type { TaskDetailRouteState } from "./task-detail-route-state";
import { deriveTaskDetailRouteView } from "./task-detail-route-view";

const TASK_ID = "task-current";
const OTHER_TASK_ID = "task-other";

function task(id: string): Task {
  return {
    id: taskId(id),
    title: id,
    description: "",
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    primary_session_id: sessionId("session-1"),
    state: "COMPLETED",
    archived_at: null,
    priority: "medium",
    position: 0,
    repositories: [],
    created_at: "2026-10-27T00:00:00Z",
    updated_at: "2026-10-27T00:00:00Z",
  } as Task;
}

function loadingState(): TaskDetailRouteState {
  return { routeKey: TASK_ID, status: "loading", data: null };
}

function errorState(temporary: boolean): TaskDetailRouteState {
  return { routeKey: TASK_ID, status: "error", data: null, temporary };
}

function loadedState(id: string): TaskDetailRouteState {
  return {
    routeKey: TASK_ID,
    status: "loaded",
    forceMergeSession: false,
    data: {
      task: task(id),
      sessionId: "session-1",
      initialState: {},
      initialTerminals: [],
    },
  } as unknown as TaskDetailRouteState;
}

function projection(id = TASK_ID) {
  return { task: task(id), sessionId: "session-1" };
}

function derive(
  currentRouteState: TaskDetailRouteState,
  projectionValue = projection(),
  readState?: TaskNavigationReadSnapshot,
) {
  return deriveTaskDetailRouteView({
    currentRouteState,
    previousLoadedRoute: null,
    routeKey: TASK_ID,
    taskId: TASK_ID,
    routeSessionId: undefined,
    selectedSessionId: "session-1",
    projection: projectionValue,
    readState,
  });
}

function readSnapshot(
  phase: TaskNavigationReadSnapshot["phase"],
  temporary: boolean,
): TaskNavigationReadSnapshot {
  return { phase, attempt: 1, temporary, revision: 3, cycle: 2 };
}

describe("deriveTaskDetailRouteView projection continuity", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.7
  it("retains the current projection during loading and a temporary failure", () => {
    for (const state of [loadingState(), errorState(true)]) {
      const view = derive(state);
      expect(view.task?.id).toBe(TASK_ID);
      expect(view.activeSessionId).toBe("session-1");
      expect(view.data).toBeNull();
      expect(view.initialState).toBeNull();
      expect(view.displayedRouteKey).toBeNull();
      expect(view.showShell).toBe(true);
      expect(view.showInitialLoading).toBe(false);
      expect(view.isLoadingOverPreviousRoute).toBe(false);
    }
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.7
  it("keeps the projection while the current generation is still retrying", () => {
    const view = derive(errorState(true), projection(), readSnapshot("retrying", true));
    expect(view.task?.id).toBe(TASK_ID);
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.5
  it("drops the retained projection when a later current-generation read fails permanently", () => {
    const view = derive(errorState(true), projection(), readSnapshot("failed", false));
    expect(view.task).toBeNull();
    expect(view.data).toBeNull();
    expect(view.showShell).toBe(true);
  });

  it("does not retain content for a permanent failure", () => {
    const view = derive(errorState(false));
    expect(view.task).toBeNull();
    expect(view.showShell).toBe(true);
    expect(view.showInitialLoading).toBe(false);
  });

  it("does not use a previous route snapshot for the unavailable destination", () => {
    const view = deriveTaskDetailRouteView({
      currentRouteState: errorState(true),
      previousLoadedRoute: loadedState(OTHER_TASK_ID),
      routeKey: TASK_ID,
      taskId: TASK_ID,
      routeSessionId: undefined,
      selectedSessionId: "session-1",
      projection: undefined,
    });
    expect(view.task).toBeNull();
    expect(view.data).toBeNull();
    expect(view.initialState).toBeNull();
  });

  it("keeps the loaded same-task route ahead of its projection", () => {
    const view = deriveTaskDetailRouteView({
      currentRouteState: loadedState(TASK_ID),
      previousLoadedRoute: null,
      routeKey: TASK_ID,
      taskId: TASK_ID,
      routeSessionId: undefined,
      selectedSessionId: "session-1",
      projection: projection(),
    });
    expect(view.currentRouteStatus).toBe("loaded");
    expect(view.data?.task.id).toBe(TASK_ID);
    expect(view.showInitialLoading).toBe(false);
  });
});
