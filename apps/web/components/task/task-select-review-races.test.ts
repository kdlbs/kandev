import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { replaceTaskUrl } from "@/lib/links";
import { selectTaskWithLayout } from "./task-select-helpers";

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
  releaseLayoutToDefault: vi.fn(),
}));
vi.mock("@/lib/links", () => ({ replaceTaskUrl: vi.fn() }));

const TASK_ID = "task-pending";
const SESSION_ID = "session-pending";
const ORIGINAL_TASK_ID = "task-original";
const ORIGINAL_SESSION_ID = "session-original";

type PendingTask = {
  id: string;
  primarySessionId: string | null;
  taskPendingAction: "clarification";
};

function makeHarness(task: PendingTask) {
  const state = {
    tasks: {
      activeTaskId: ORIGINAL_TASK_ID,
      activeSessionId: ORIGINAL_SESSION_ID as string | null,
      lastSessionByTaskId: {},
    },
    taskPRs: { byTaskId: {} },
    environmentIdBySessionId: {},
    taskSessions: { items: {} },
    kanban: { tasks: [task] },
    kanbanMulti: { snapshots: {} },
  };
  const store = {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(() => vi.fn()),
  } as unknown as StoreApi<AppState>;
  const setActiveTask = vi.fn((taskId: string) => {
    state.tasks.activeTaskId = taskId;
    state.tasks.activeSessionId = null;
  });
  return { state, store, setActiveTask };
}

function deferredSessionLoad() {
  let resolveLoad: (sessions: TaskSession[]) => void = () => undefined;
  let rejectLoad: (error: Error) => void = () => undefined;
  const loadTaskSessionsForTask = vi.fn(
    () =>
      new Promise<TaskSession[]>((resolve, reject) => {
        resolveLoad = resolve;
        rejectLoad = reject;
      }),
  );
  return {
    loadTaskSessionsForTask,
    resolveLoad: (sessions: TaskSession[]) => resolveLoad(sessions),
    rejectLoad: (error: Error) => rejectLoad(error),
  };
}

async function flushSelection() {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("selectTaskWithLayout review races", () => {
  beforeEach(() => vi.clearAllMocks());

  it("leaves a legacy pending selection inert when its task projection disappears", async () => {
    const task: PendingTask = {
      id: TASK_ID,
      primarySessionId: SESSION_ID,
      taskPendingAction: "clarification",
    };
    const { state, store, setActiveTask } = makeHarness(task);
    const switchToSession = vi.fn();
    const { loadTaskSessionsForTask, resolveLoad } = deferredSessionLoad();

    selectTaskWithLayout({
      taskId: TASK_ID,
      task,
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask,
      setPreparingTaskId: vi.fn(),
    });
    state.kanban.tasks = [];
    resolveLoad([
      {
        id: SESSION_ID,
        task_id: TASK_ID,
        state: "WAITING_FOR_INPUT",
        pending_action: "clarification",
      } as TaskSession,
    ]);
    await flushSelection();

    expect(switchToSession).not.toHaveBeenCalled();
    expect(setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrl).not.toHaveBeenCalledWith(TASK_ID);
  });

  for (const primarySessionId of [SESSION_ID, null]) {
    it(`ignores an aborted ${primarySessionId ? "primary" : "sessionless"} forced load`, async () => {
      const task: PendingTask = {
        id: TASK_ID,
        primarySessionId,
        taskPendingAction: "clarification",
      };
      const { state, store, setActiveTask } = makeHarness(task);
      const switchToSession = vi.fn();
      const { loadTaskSessionsForTask, rejectLoad } = deferredSessionLoad();

      selectTaskWithLayout({
        taskId: TASK_ID,
        task,
        store,
        switchToSession,
        loadTaskSessionsForTask,
        setActiveTask,
        setPreparingTaskId: vi.fn(),
      });
      state.tasks.activeTaskId = TASK_ID;
      state.tasks.activeSessionId = SESSION_ID;
      const abortError = new Error("superseded");
      abortError.name = "AbortError";
      rejectLoad(abortError);
      await flushSelection();

      expect(switchToSession).not.toHaveBeenCalled();
      expect(setActiveTask).not.toHaveBeenCalled();
      expect(replaceTaskUrl).not.toHaveBeenCalledWith(TASK_ID);
    });
  }
});
