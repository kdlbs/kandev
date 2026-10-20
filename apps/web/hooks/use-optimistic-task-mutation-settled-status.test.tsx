import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  TaskOptimisticContextProvider,
  useOptimisticTaskMutation,
} from "./use-optimistic-task-mutation";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { __resetOfficeTaskContentSyncForTests } from "@/lib/state/office-task-content-sync";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ updateTask: vi.fn() }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  __resetOfficeTaskContentSyncForTests();
});

const TS = "2026-05-01T00:00:00Z";

const baseTask: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "in_progress",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user",
  createdAt: TS,
  updatedAt: TS,
};

const baseOfficeTask: OfficeTask = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "in_progress",
  priority: "medium",
  createdAt: TS,
  updatedAt: TS,
};

function setup() {
  const state = { task: baseTask };
  let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
  let store: ReturnType<typeof useAppStoreApi> | null = null;
  const ctxValue = {
    task: baseTask,
    applyPatch: (patch: Partial<Task>) => {
      state.task = { ...state.task, ...patch };
    },
    restore: (snapshot: Task) => {
      state.task = snapshot;
    },
  };
  function Probe() {
    store = useAppStoreApi();
    mutate = useOptimisticTaskMutation();
    return null;
  }
  function Seed({ children }: { children: ReactNode }) {
    useAppStoreApi().getState().setTasks([baseOfficeTask]);
    return <>{children}</>;
  }
  render(
    <StateProvider>
      <Seed>
        <TaskOptimisticContextProvider value={ctxValue}>
          <Probe />
        </TaskOptimisticContextProvider>
      </Seed>
    </StateProvider>,
  );
  const storeStatus = () =>
    store!.getState().office.tasks.items.find((t) => t.id === "t-1")?.status;
  return { state, mutate: () => mutate!, storeStatus };
}

describe("useOptimisticTaskMutation settled status", () => {
  // @covers AC-OFFICE-HUMAN-DONE-001.2
  it("settles on the status the backend reports instead of the optimistic one", async () => {
    const { state, mutate, storeStatus } = setup();

    await act(async () => {
      await mutate()("t-1", { status: "done" }, async () => ({ status: "in_progress" }));
    });

    expect(state.task.status).toBe("in_progress");
    expect(storeStatus()).toBe("in_progress");
  });

  it("keeps the optimistic status when the backend reports the same one", async () => {
    const { state, mutate, storeStatus } = setup();

    await act(async () => {
      await mutate()("t-1", { status: "done" }, async () => ({ status: "done" }));
    });

    expect(state.task.status).toBe("done");
    expect(storeStatus()).toBe("done");
  });
});
