import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { ApiError } from "@/lib/api/client";
import { BulkMoveApiError, type BulkMoveErrorResponse } from "@/lib/api/domains/task-bulk-move-api";
import { bulkMoveSelectedTasks, getBulkMoveApiError } from "@/lib/api";

const mockToast = vi.fn();
const mockTranslation = (key: string, options?: Record<string, unknown>) => {
  const values: Record<string, string> = {
    "task:movedTasksToStep": "Moved task to step",
    "task:movedTasksStepDescription": "The task is now in the selected step.",
    "task:bulkMoveNoTasksMoved": "No tasks were moved",
    "task:bulkMoveRefusalLine": "{{title}}: {{reason}}",
    "task:bulkMoveApprovalPending": "Approval pending ({{approvers}})",
    "task:bulkMoveRefusedDescription": "{{items}}",
    "task:bulkMovePartialTitle": "Moved {{moved}} of {{total}} tasks",
    "task:bulkMovePartialDescription":
      "{{task}} was not moved: {{reason}}. Remaining tasks were not moved.",
    "task:taskMoveErrorGeneric": "The task could not be moved. Try again.",
    "task:taskMoveErrorApprovalsPending":
      "The task cannot be completed until its approvers approve it.",
  };
  return Object.entries(options ?? {}).reduce(
    (text, [name, value]) => text.replaceAll(`{{${name}}}`, String(value)),
    values[key] ?? key,
  );
};

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api", () => ({
  bulkMoveSelectedTasks: vi.fn(),
  getBulkMoveApiError: vi.fn(() => null),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: mockTranslation, i18n: { language: "en" } }),
}));

const mockBulkMoveSelectedTasks = vi.mocked(bulkMoveSelectedTasks);
const mockGetBulkMoveApiError = vi.mocked(getBulkMoveApiError);

function createBulkMoveApiError(result: BulkMoveErrorResponse) {
  return new BulkMoveApiError(new ApiError("bulk move failed", 409, result), result);
}

describe("useTaskWorkflowMove", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetBulkMoveApiError.mockReturnValue(null);
  });

  it("returns the moved task IDs after showing the success toast", async () => {
    const response = { moved_count: 1, moved_task_ids: ["task-1"] };
    mockBulkMoveSelectedTasks.mockResolvedValue(response);
    const { result } = renderHook(() => useTaskWorkflowMove());

    let moved: typeof response | undefined;
    await act(async () => {
      moved = await result.current(["task-1"], "workflow-1", "step-2", "step");
    });

    expect(moved).toEqual(response);
    expect(mockToast).toHaveBeenCalledWith({
      title: "Moved task to step",
      description: "The task is now in the selected step.",
      variant: "success",
    });
  });

  it("shows task titles and reasons for a whole-batch refusal", async () => {
    const error = new Error("bulk move refused");
    mockBulkMoveSelectedTasks.mockRejectedValue(error);
    mockGetBulkMoveApiError.mockReturnValue(
      createBulkMoveApiError({
        code: "bulk_move_refused",
        moved_task_ids: [],
        refusals: [
          {
            task_id: "task-1",
            task_title: "Fix login",
            code: "task_approvals_pending",
            message: "task approvals pending",
            pending_approvers: [{ agent_profile_id: "agent-ceo", name: "CEO" }],
          },
        ],
      }),
    );
    const { result } = renderHook(() => useTaskWorkflowMove());

    await expect(result.current(["task-1"], "workflow-1", "step-2", "step")).rejects.toThrow(error);

    expect(mockToast).toHaveBeenCalledWith({
      title: "No tasks were moved",
      description: "Fix login: Approval pending (CEO)",
      variant: "error",
    });
  });

  it("reports partial moves using the committed task IDs", async () => {
    const error = new Error("bulk move partially refused");
    mockBulkMoveSelectedTasks.mockRejectedValue(error);
    mockGetBulkMoveApiError.mockReturnValue(
      createBulkMoveApiError({
        code: "bulk_move_partial",
        moved_task_ids: ["task-1"],
        refusals: [
          {
            task_id: "task-2",
            task_title: "Update docs",
            code: "task_approvals_pending",
            message: "task approvals pending",
            pending_approvers: [{ agent_profile_id: "agent-ceo", name: "CEO" }],
          },
        ],
      }),
    );
    const { result } = renderHook(() => useTaskWorkflowMove());

    await expect(
      result.current(["task-1", "task-2", "task-3"], "workflow-1", "step-2", "step"),
    ).rejects.toThrow(error);

    expect(mockToast).toHaveBeenCalledWith({
      title: "Moved 1 of 3 tasks",
      description:
        "Update docs was not moved: Approval pending (CEO). Remaining tasks were not moved.",
      variant: "error",
    });
  });
});
