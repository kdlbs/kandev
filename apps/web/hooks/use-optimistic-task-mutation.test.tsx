import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import {
  TaskOptimisticContextProvider,
  useOptimisticTaskMutation,
} from "./use-optimistic-task-mutation";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";

const WS_ID = "ws-1";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

import { toast } from "sonner";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const TS = "2026-05-01T00:00:00Z";

const baseTask: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "todo",
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
  status: "todo",
  priority: "medium",
  createdAt: TS,
  updatedAt: TS,
};

function makeHarness(initialTask: Task, initialOffice: OfficeTask | null) {
  const state = {
    task: initialTask,
    patches: [] as Partial<Task>[],
    restored: [] as Task[],
  };
  const ctxValue = {
    task: initialTask,
    applyPatch: (patch: Partial<Task>) => {
      state.patches.push(patch);
      state.task = { ...state.task, ...patch };
    },
    restore: (snapshot: Task) => {
      state.restored.push(snapshot);
      state.task = snapshot;
    },
  };
  // Seed the office tasks TQ cache (the list view's source) so the
  // optimistic patch has a task to update. Mutation patches the flat
  // `qk.office.tasks(wsId)` cache. gcTime is non-zero here so the
  // observer-less cache entry survives long enough to assert on after the
  // mutation settles.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  if (initialOffice) {
    client.setQueryData(qk.office.tasks(WS_ID), [initialOffice]);
  }
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <StateProvider initialState={{ workspaces: { activeId: WS_ID } }}>
          <TaskOptimisticContextProvider value={ctxValue}>{children}</TaskOptimisticContextProvider>
        </StateProvider>
      </QueryClientProvider>
    );
  }
  return { Wrapper, state, client };
}

function HookProbe({
  onReady,
}: {
  onReady: (mutate: ReturnType<typeof useOptimisticTaskMutation>) => void;
}) {
  const mutate = useOptimisticTaskMutation();
  onReady(mutate);
  return null;
}

describe("useOptimisticTaskMutation", () => {
  it("applies the patch and keeps it on success", async () => {
    const { Wrapper, state, client } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    expect(mutate).not.toBeNull();
    const apiCall = vi.fn().mockResolvedValue({ ok: true });
    await act(async () => {
      await mutate!("t-1", { priority: "high" }, apiCall);
    });
    expect(apiCall).toHaveBeenCalledTimes(1);
    expect(state.patches).toEqual([{ priority: "high" }]);
    expect(state.restored).toHaveLength(0);
    expect(state.task.priority).toBe("high");
    // The office tasks TQ cache was patched optimistically.
    const cached = client.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(cached?.[0]?.priority).toBe("high");
  });

  it("rolls back local + cache state and toasts on api failure", async () => {
    const { Wrapper, state, client } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    const apiCall = vi.fn().mockRejectedValue(new Error("nope"));
    await act(async () => {
      await expect(mutate!("t-1", { priority: "high" }, apiCall)).rejects.toThrow("nope");
    });
    expect(state.patches).toEqual([{ priority: "high" }]);
    expect(state.restored).toHaveLength(1);
    expect(state.restored[0]).toEqual(baseTask);
    expect(toast.error).toHaveBeenCalledWith("nope");
    // The optimistic cache patch was rolled back to the original priority.
    const cached = client.getQueryData<OfficeTask[]>(qk.office.tasks(WS_ID));
    expect(cached?.[0]?.priority).toBe("medium");
  });

  it("uses a generic error message when the rejection isn't an Error", async () => {
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    const apiCall = vi.fn().mockRejectedValue("boom");
    await act(async () => {
      await expect(mutate!("t-1", { priority: "high" }, apiCall)).rejects.toBe("boom");
    });
    expect(toast.error).toHaveBeenCalledWith("Update failed");
  });

  it("works when the office store has no entry for the task", async () => {
    const { Wrapper, state } = makeHarness(baseTask, null);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    const apiCall = vi.fn().mockResolvedValue({ ok: true });
    await act(async () => {
      await mutate!("t-1", { status: "done" }, apiCall);
    });
    expect(state.patches).toEqual([{ status: "done" }]);
    expect(state.task.status).toBe("done");
  });
});
