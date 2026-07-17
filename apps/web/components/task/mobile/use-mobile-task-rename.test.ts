import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useMobileTaskRename } from "./use-mobile-task-rename";

const renameTaskById = vi.fn();

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({
    moveTaskById: vi.fn(),
    deleteTaskById: vi.fn(),
    archiveTaskById: vi.fn(),
    renameTaskById,
  }),
}));

describe("useMobileTaskRename", () => {
  beforeEach(() => {
    renameTaskById.mockReset();
    renameTaskById.mockResolvedValue(undefined);
  });

  it("renames the selected task and clears the dialog target", async () => {
    const { result } = renderHook(() => useMobileTaskRename());

    act(() => result.current.handleRenameTask("task-1", "Old title"));
    await act(() => result.current.handleRenameSubmit("New title"));

    expect(renameTaskById).toHaveBeenCalledWith("task-1", "New title");
    expect(result.current.renamingTask).toBeNull();
  });
});
