import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { isTaskRemovalPending } from "@/lib/state/task-removal";

const api = vi.hoisted(() => ({
  archiveTask: vi.fn(),
  softNavigate: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: api.toast, dismissToast: vi.fn() }),
}));

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  archiveTask: (...args: unknown[]) => api.archiveTask(...args),
}));

vi.mock("@/lib/routing/client-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/routing/client-router")>()),
  softNavigate: (...args: unknown[]) => api.softNavigate(...args),
}));

import { useArchiveActions } from "./task-session-sidebar-archive-actions";

const WORKSPACE_ID = "workspace-1";
const TASK_A = "task-a";
const TASK_B = "task-b";
const TASK_C = "task-c";

let store: ReturnType<typeof createAppStore>;

function makeTask(id: string, position: number) {
  return {
    id,
    workspaceId: WORKSPACE_ID,
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    title: id,
    position,
    primarySessionId: `session-${id}`,
  };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function makeStore() {
  const created = createAppStore();
  created.setState((state) => ({
    ...state,
    workspaces: { ...state.workspaces, activeId: WORKSPACE_ID },
  }));
  const tasks = [makeTask(TASK_A, 0), makeTask(TASK_B, 1), makeTask(TASK_C, 2)];
  created.setState((state) => ({
    ...state,
    kanban: { ...state.kanban, tasks },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        "workflow-1": {
          workflowId: "workflow-1",
          workflowName: "Workflow",
          steps: [],
          tasks,
        },
      },
    },
  }));
  created.getState().setActiveSession(TASK_A, `session-${TASK_A}`);
  return created;
}

function renderArchiveActions() {
  return renderHook(() =>
    useArchiveActions(store as Parameters<typeof useArchiveActions>[0], [], WORKSPACE_ID),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  store = makeStore();
});

describe("useArchiveActions independent archive controls", () => {
  it("keeps a target busy until its own operation releases, not the latest request", async () => {
    const firstArchive = deferred<void>();
    const secondArchive = deferred<void>();
    api.archiveTask.mockImplementation((taskId: string) =>
      taskId === TASK_A ? firstArchive.promise : secondArchive.promise,
    );
    const { result } = renderArchiveActions();

    act(() => result.current.handleArchiveTask(TASK_A, { cascade: false }));
    await waitFor(() =>
      expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_A, WORKSPACE_ID)).toBe(true),
    );

    act(() => result.current.handleArchiveTask(TASK_B));
    expect(result.current.archivingTask?.id).toBe(TASK_B);
    expect(result.current.isArchiving).toBe(false);

    let confirmation!: Promise<void>;
    act(() => {
      confirmation = result.current.handleArchiveConfirm({ cascade: false });
    });
    await waitFor(() =>
      expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_B, WORKSPACE_ID)).toBe(true),
    );
    expect(result.current.isArchiving).toBe(true);

    await act(async () => {
      firstArchive.resolve();
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_A, WORKSPACE_ID)).toBe(false),
    );
    expect(result.current.archivingTask?.id).toBe(TASK_B);
    expect(result.current.isArchiving).toBe(true);

    await act(async () => {
      secondArchive.resolve();
      await confirmation;
    });
    expect(result.current.archivingTask).toBeNull();
    expect(result.current.isArchiving).toBe(false);
  });

  it("suppresses a duplicate archive of a target that is already pending", async () => {
    const archive = deferred<void>();
    api.archiveTask.mockReturnValue(archive.promise);
    const { result } = renderArchiveActions();

    act(() => result.current.handleArchiveTask(TASK_A, { cascade: false }));
    act(() => result.current.handleArchiveTask(TASK_A, { cascade: false }));
    await waitFor(() => expect(api.archiveTask).toHaveBeenCalledOnce());
    expect(api.archiveTask).toHaveBeenCalledWith(TASK_A, { cascade: false });

    await act(async () => {
      archive.resolve();
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_A, WORKSPACE_ID)).toBe(false),
    );
  });

  it("does not report a workspace-scoped archive target as busy elsewhere", async () => {
    api.archiveTask.mockReturnValue(new Promise<void>(() => undefined));
    const { result } = renderArchiveActions();
    act(() => result.current.handleArchiveTask(TASK_B));
    let confirmation!: Promise<void>;
    act(() => {
      confirmation = result.current.handleArchiveConfirm({ cascade: false });
    });
    await waitFor(() => expect(result.current.isArchiving).toBe(true));
    void confirmation;

    const otherWorkspace = renderHook(() =>
      useArchiveActions(store as Parameters<typeof useArchiveActions>[0], [], "workspace-2"),
    );
    act(() => otherWorkspace.result.current.handleArchiveTask(TASK_B));
    expect(otherWorkspace.result.current.isArchiving).toBe(false);
    expect(result.current.isArchiving).toBe(true);
  });
});
