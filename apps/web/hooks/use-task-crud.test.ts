import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const deleteTaskById = vi.fn();
const archiveTaskById = vi.fn();
const runTaskRemoval = vi.fn();
const store = { getState: vi.fn() };

vi.mock("./use-task-actions", () => ({
  useTaskActions: () => ({ deleteTaskById, archiveTaskById }),
}));

vi.mock("./use-task-removal", () => ({
  useTaskRemovalSuccessNotifier: () => vi.fn(),
  useTaskRemoval: () => ({ runTaskRemoval }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
}));

import { useTaskCRUD } from "./use-task-crud";

const TASK = {
  id: "task-1",
  title: "Task",
  workflowStepId: "step-1",
  state: "TODO",
  position: 0,
} as const;

beforeEach(() => {
  vi.clearAllMocks();
  deleteTaskById.mockResolvedValue(undefined);
  archiveTaskById.mockResolvedValue(undefined);
  runTaskRemoval.mockImplementation(
    async (_action: string, request: { mutate: () => Promise<void> }) => {
      await request.mutate();
      return {
        skipped: false,
        operationToken: "removal-1",
        switchedTaskId: null,
        succeededTaskIds: [TASK.id],
        failedTaskIds: [],
      };
    },
  );
});

describe("useTaskCRUD removal actions", () => {
  it("publishes delete intent through the coordinator before the mutation", async () => {
    const order: string[] = [];
    runTaskRemoval.mockImplementationOnce(
      async (_action: string, request: { mutate: () => Promise<void> }) => {
        order.push("coordinator");
        await request.mutate();
        return {
          skipped: false,
          operationToken: "removal-1",
          switchedTaskId: null,
          succeededTaskIds: [TASK.id],
          failedTaskIds: [],
        };
      },
    );
    deleteTaskById.mockImplementationOnce(async () => {
      order.push("mutation");
    });

    const { result } = renderHook(() => useTaskCRUD());
    await act(async () => {
      await result.current.handleDelete(TASK, { cascade: true });
    });

    expect(order).toEqual(["coordinator", "mutation"]);
    expect(runTaskRemoval).toHaveBeenCalledWith(
      "delete",
      { taskId: TASK.id, mutate: expect.any(Function) },
      { cascade: true },
    );
  });

  it("uses the same coordinator for archive actions", async () => {
    const { result } = renderHook(() => useTaskCRUD());

    await act(async () => {
      await result.current.handleArchive(TASK, { cascade: false });
    });

    expect(runTaskRemoval).toHaveBeenCalledWith(
      "archive",
      { taskId: TASK.id, mutate: expect.any(Function) },
      { cascade: false },
    );
  });
});
