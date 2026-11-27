import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { launchSession } from "@/lib/services/session-launch-service";
import { performLayoutSwitch, releaseLayoutToDefault } from "@/lib/state/dockview-store";
import { selectTaskWithLayout } from "./task-select-helpers";

const ARCHIVED_TASK_ID = "archived-task";
const ARCHIVED_SESSION_ID = "archived-session";
const OLD_SESSION_ID = "sess-old";
const COMPLETED_SESSION_STATE = "COMPLETED";

const { dockviewState } = vi.hoisted(() => ({
  dockviewState: { api: null as unknown, buildDefaultLayout: vi.fn() },
}));

vi.mock("@/lib/services/session-launch-service", () => ({ launchSession: vi.fn() }));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildPrepareRequest: vi.fn(() => ({ request: { taskId: "task-new" } })),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
  releaseLayoutToDefault: vi.fn(),
  useDockviewStore: { getState: () => dockviewState },
}));
vi.mock("@/lib/links", () => ({ replaceTaskUrl: vi.fn() }));

function makeKanbanStore(activeSessionId: string | null): StoreApi<AppState> {
  const state = {
    tasks: { activeTaskId: null, activeSessionId, lastSessionByTaskId: {} },
    taskPRs: { byTaskId: {} as Record<string, unknown[]> },
    environmentIdBySessionId: activeSessionId ? { [activeSessionId]: "env-old" } : {},
    taskSessions: { items: {} as Record<string, TaskSession> },
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

async function flushTaskSelection(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("selectTaskWithLayout archived tasks", () => {
  it("selects the saved primary session without preparing a new one", async () => {
    const store = makeKanbanStore(OLD_SESSION_ID);
    const loadTaskSessionsForTask = vi.fn(async () => [
      {
        id: ARCHIVED_SESSION_ID,
        task_id: ARCHIVED_TASK_ID,
        state: COMPLETED_SESSION_STATE,
        is_primary: true,
      } as TaskSession,
    ]);
    const switchToSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigateToTask = vi.fn();

    selectTaskWithLayout({
      taskId: ARCHIVED_TASK_ID,
      task: { isArchived: true, primarySessionId: ARCHIVED_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
      navigateToTask,
    });
    await flushTaskSelection();

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(ARCHIVED_TASK_ID);
    expect(setActiveTask).not.toHaveBeenCalled();
    expect(switchToSession).toHaveBeenCalledWith(
      ARCHIVED_TASK_ID,
      ARCHIVED_SESSION_ID,
      OLD_SESSION_ID,
    );
    expect(navigateToTask).toHaveBeenCalledWith(ARCHIVED_TASK_ID, ARCHIVED_SESSION_ID);
    expect(launchSession).not.toHaveBeenCalled();
  });

  it("opens a sessionless archived task without preparing a replacement", async () => {
    const store = makeKanbanStore(OLD_SESSION_ID);
    const loadTaskSessionsForTask = vi.fn(async () => []);
    const setActiveTask = vi.fn();
    const navigateToTask = vi.fn();

    selectTaskWithLayout({
      taskId: ARCHIVED_TASK_ID,
      task: { isArchived: true },
      store,
      switchToSession: vi.fn(),
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
      navigateToTask,
    });
    await flushTaskSelection();

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(ARCHIVED_TASK_ID);
    expect(releaseLayoutToDefault).toHaveBeenCalledWith("env-old");
    expect(setActiveTask).toHaveBeenCalledWith(ARCHIVED_TASK_ID);
    expect(navigateToTask).toHaveBeenCalledWith(ARCHIVED_TASK_ID);
    expect(launchSession).not.toHaveBeenCalled();
    expect(performLayoutSwitch).not.toHaveBeenCalled();
  });
});
