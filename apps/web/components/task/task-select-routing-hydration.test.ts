import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { fetchTaskSession } from "@/lib/api/domains/session-api";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { selectTaskWithLayout } from "./task-select-helpers";

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: vi.fn(),
}));
vi.mock("@/lib/services/session-launch-helpers", () => ({
  buildPrepareRequest: vi.fn(() => ({ request: { taskId: "task-new" } })),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
  releaseLayoutToDefault: vi.fn(),
  useDockviewStore: { getState: () => ({ api: null, buildDefaultLayout: vi.fn() }) },
}));
vi.mock("@/lib/state/layout-manager", () => ({
  INTENT_PR_REVIEW: "pr-review",
}));
vi.mock("@/lib/links", () => ({
  replaceTaskUrl: vi.fn(),
}));
vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(async () => ({ session: null })),
}));

const SELECT_TASK_ID = "task-A";
const PRIMARY_SESSION_ID = "sess-primary";
const OTHER_TASK_SESSION_ID = "sess-other-task";
const ENV_A = "env-A";
const ENV_B = "env-B";

function makeKanbanStore(args: {
  activeSessionId: string | null;
  envIds: Record<string, string>;
  sessionTaskIds?: Record<string, string>;
}): StoreApi<AppState> {
  const items: Record<string, Partial<TaskSession> & Pick<TaskSession, "id" | "task_id">> = {};
  for (const [sid, tid] of Object.entries(args.sessionTaskIds ?? {})) {
    items[sid] = {
      id: sessionId(sid),
      task_id: taskId(tid),
      task_environment_id: args.envIds[sid],
      is_passthrough: false,
    };
  }
  const state = {
    tasks: {
      activeTaskId: null,
      activeSessionId: args.activeSessionId,
      lastSessionByTaskId: {},
    },
    taskPRs: { byTaskId: {} as Record<string, unknown[]> },
    environmentIdBySessionId: args.envIds,
    taskSessions: { items },
    setTaskSession: vi.fn((session: TaskSession) => {
      items[session.id] = session;
    }),
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

describe("selectTaskWithLayout — session routing hydration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("hydrates before switching when the target session has only partial WS fields", async () => {
    let resolveLoad: (sessions: TaskSession[]) => void = () => {};
    const loadPromise = new Promise<TaskSession[]>((resolve) => {
      resolveLoad = resolve;
    });
    const loadTaskSessionsForTask = vi.fn((_: string) => loadPromise);
    const switchToSession = vi.fn();
    const store = makeKanbanStore({
      activeSessionId: OTHER_TASK_SESSION_ID,
      envIds: { [OTHER_TASK_SESSION_ID]: ENV_B, [PRIMARY_SESSION_ID]: ENV_A },
      sessionTaskIds: { [PRIMARY_SESSION_ID]: SELECT_TASK_ID },
    });
    const state = store.getState();
    state.taskSessions.items[PRIMARY_SESSION_ID] = {
      id: PRIMARY_SESSION_ID,
      task_id: SELECT_TASK_ID,
      task_environment_id: ENV_A,
    } as TaskSession;

    selectTaskWithLayout({
      taskId: SELECT_TASK_ID,
      task: { primarySessionId: PRIMARY_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(SELECT_TASK_ID);
    expect(switchToSession).not.toHaveBeenCalled();

    resolveLoad([
      {
        id: PRIMARY_SESSION_ID,
        task_id: SELECT_TASK_ID,
        task_environment_id: ENV_A,
        is_passthrough: true,
      } as TaskSession,
    ]);
    await loadPromise;
    await Promise.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(switchToSession).toHaveBeenCalledWith(
      SELECT_TASK_ID,
      PRIMARY_SESSION_ID,
      OTHER_TASK_SESSION_ID,
    );
  });

  it("fetches session detail before switching when loaded task sessions still lack routing fields", async () => {
    const loadTaskSessionsForTask = vi.fn(async () => [
      {
        id: PRIMARY_SESSION_ID,
        task_id: SELECT_TASK_ID,
        task_environment_id: ENV_A,
        is_primary: true,
      } as TaskSession,
    ]);
    vi.mocked(fetchTaskSession).mockResolvedValueOnce({
      session: {
        id: PRIMARY_SESSION_ID,
        task_id: SELECT_TASK_ID,
        task_environment_id: ENV_A,
        is_passthrough: true,
      } as TaskSession,
    });
    const switchToSession = vi.fn();
    const store = makeKanbanStore({
      activeSessionId: OTHER_TASK_SESSION_ID,
      envIds: { [OTHER_TASK_SESSION_ID]: ENV_B, [PRIMARY_SESSION_ID]: ENV_A },
      sessionTaskIds: { [PRIMARY_SESSION_ID]: SELECT_TASK_ID },
    });
    const state = store.getState();
    state.taskSessions.items[PRIMARY_SESSION_ID] = {
      id: PRIMARY_SESSION_ID,
      task_id: SELECT_TASK_ID,
      task_environment_id: ENV_A,
    } as TaskSession;

    selectTaskWithLayout({
      taskId: SELECT_TASK_ID,
      task: { primarySessionId: PRIMARY_SESSION_ID },
      store,
      switchToSession,
      loadTaskSessionsForTask,
      setActiveTask: vi.fn(),
      setPreparingTaskId: vi.fn(),
    });

    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(fetchTaskSession).toHaveBeenCalledWith(PRIMARY_SESSION_ID, { cache: "no-store" });
    expect(state.setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ id: PRIMARY_SESSION_ID, is_passthrough: true }),
    );
    expect(switchToSession).toHaveBeenCalledWith(
      SELECT_TASK_ID,
      PRIMARY_SESSION_ID,
      OTHER_TASK_SESSION_ID,
    );
  });
});
