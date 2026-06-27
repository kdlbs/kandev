import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, type ReactNode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import { ApiError } from "@/lib/api/client";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { BlockersPicker, formatBlockerCycleMessage } from "./blockers-picker";
import type { Task } from "@/app/office/tasks/[id]/types";

// Hoisted mocks so the API module is replaced before the component imports it.
const addBlockerMock = vi.hoisted(() => vi.fn());
const removeBlockerMock = vi.hoisted(() => vi.fn());
const listTasksMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue({
    tasks: [{ id: "t-2", identifier: "TASK-2", title: "Beta", workspaceId: "ws-1" }],
  }),
);
const toastErrorMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return {
    ...actual,
    addTaskBlocker: addBlockerMock,
    removeTaskBlocker: removeBlockerMock,
  };
});

vi.mock("@/lib/api/domains/office-tasks-api", () => ({
  listTasks: listTasksMock,
}));

vi.mock("sonner", async () => {
  const actual = await vi.importActual<typeof import("sonner")>("sonner");
  return {
    ...actual,
    toast: { ...actual.toast, error: toastErrorMock },
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

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
  createdAt: "2026-05-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function SeedWorkspace() {
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  useEffect(() => {
    setActiveWorkspace("ws-1");
  }, [setActiveWorkspace]);
  return null;
}

function Wrapper({
  children,
  applyPatch,
  restore,
  candidates,
}: {
  children: ReactNode;
  applyPatch: (patch: Partial<Task>) => void;
  restore: (snapshot: Task) => void;
  candidates: OfficeTask[];
}) {
  const ctx = { task: baseTask, applyPatch, restore };
  const queryClient = createQueryClient();
  listTasksMock.mockResolvedValue({
    tasks: candidates,
    next_cursor: "",
    next_id: "",
  });
  return (
    <QueryClientProvider client={queryClient}>
      <StateProvider>
        <SeedWorkspace />
        <TaskOptimisticContextProvider value={ctx}>{children}</TaskOptimisticContextProvider>
      </StateProvider>
    </QueryClientProvider>
  );
}

describe("formatBlockerCycleMessage", () => {
  it("joins identifiers with the arrow separator", () => {
    expect(formatBlockerCycleMessage(["TASK-A", "TASK-B", "TASK-A"])).toBe(
      "Would create a blocker cycle: TASK-A → TASK-B → TASK-A",
    );
  });
});

describe("BlockersPicker", () => {
  it("rolls back the optimistic chip and toasts the cycle path on a 400 cycle response", async () => {
    addBlockerMock.mockRejectedValueOnce(
      new ApiError("would create blocker cycle", 400, {
        error: "would create blocker cycle: A → B → A",
        cycle: ["A", "B", "A"],
      }),
    );

    const applyPatch = vi.fn();
    const restore = vi.fn();
    const candidates: OfficeTask[] = [
      {
        id: "t-2",
        identifier: "TASK-2",
        title: "Beta",
      } as unknown as OfficeTask,
    ];

    render(
      <Wrapper applyPatch={applyPatch} restore={restore} candidates={candidates}>
        <BlockersPicker task={baseTask} />
      </Wrapper>,
    );

    fireEvent.click(screen.getByTestId("blockers-picker-trigger"));
    const option = await screen.findByTestId("multi-select-add-t-2");
    await act(async () => {
      fireEvent.click(option);
    });

    // The optimistic patch was applied, then rolled back on error.
    expect(applyPatch).toHaveBeenCalledWith({ blockedBy: ["t-2"] });
    expect(restore).toHaveBeenCalledWith(baseTask);

    // The toast surfaced the formatted cycle message, not the raw error.
    expect(toastErrorMock).toHaveBeenCalledWith("Would create a blocker cycle: A → B → A");
  });
});
