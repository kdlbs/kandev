import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useMobileTaskRename } from "./use-mobile-task-rename";

const renameTaskById = vi.fn();
const toast = vi.fn();

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({
    moveTaskById: vi.fn(),
    deleteTaskById: vi.fn(),
    archiveTaskById: vi.fn(),
    renameTaskById,
  }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast }),
}));

describe("useMobileTaskRename", () => {
  beforeEach(() => {
    renameTaskById.mockReset();
    renameTaskById.mockResolvedValue(undefined);
    toast.mockReset();
  });

  it("renames the selected task and clears the dialog target", async () => {
    const { result } = renderHook(() => useMobileTaskRename());

    act(() => result.current.handleRenameTask("task-1", "Old title"));
    await act(() => result.current.handleRenameSubmit("New title"));

    expect(renameTaskById).toHaveBeenCalledWith("task-1", "New title");
    expect(result.current.renamingTask).toBeNull();
  });

  it("shows an error when renaming fails", async () => {
    const error = new Error("Rename unavailable");
    renameTaskById.mockRejectedValue(error);
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const { result } = renderHook(() => useMobileTaskRename());

    act(() => result.current.handleRenameTask("task-1", "Old title"));
    await act(() => result.current.handleRenameSubmit("New title"));

    expect(toast).toHaveBeenCalledWith({
      title: "Failed to rename task",
      description: "Rename unavailable",
      variant: "error",
    });
    expect(result.current.renamingTask).toBeNull();
    consoleError.mockRestore();
  });
});
