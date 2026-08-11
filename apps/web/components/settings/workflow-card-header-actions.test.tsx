import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkflowCardHeaderActions } from "./workflow-card-header-actions";

afterEach(cleanup);

const DUPLICATE_BUTTON_TEST_ID = "duplicate-workflow-button";

const renderActions = (
  overrides: Partial<ComponentProps<typeof WorkflowCardHeaderActions>> = {},
) => {
  const defaultProps: ComponentProps<typeof WorkflowCardHeaderActions> = {
    workflowId: "workflow-1",
    setExportYaml: vi.fn(),
    setExportOpen: vi.fn(),
    toast: vi.fn(),
    onDeleteClick: vi.fn().mockResolvedValue(undefined),
    onDuplicateClick: vi.fn().mockResolvedValue(undefined),
    deleteDisabled: false,
    exportDisabled: false,
    duplicateDisabled: false,
    duplicateLoading: false,
    readOnly: false,
  };

  return render(
    <TooltipProvider>
      <WorkflowCardHeaderActions {...defaultProps} {...overrides} />
    </TooltipProvider>,
  );
};

describe("WorkflowCardHeaderActions", () => {
  it("surfaces failures when deleting a temporary workflow", async () => {
    const failure = new Error("cleanup failed");
    const toast = vi.fn();

    renderActions({
      workflowId: "temp-workflow-1",
      toast,
      onDeleteClick: vi.fn().mockRejectedValue(failure),
      exportDisabled: true,
      duplicateDisabled: true,
    });

    fireEvent.click(screen.getByRole("button", { name: "Delete Workflow" }));

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith({
        title: "Failed to delete workflow",
        description: "cleanup failed",
        variant: "error",
      }),
    );
  });

  it("shows the duplicate action in the card action row", () => {
    renderActions();

    expect(screen.getByTestId(DUPLICATE_BUTTON_TEST_ID)).toBeTruthy();
  });

  it("invokes duplication and exposes the save-first disabled reason", () => {
    const onDuplicateClick = vi.fn().mockResolvedValue(undefined);
    const reason = "Save the workflow before duplicating it.";

    renderActions({ onDuplicateClick });

    fireEvent.click(screen.getByTestId(DUPLICATE_BUTTON_TEST_ID));
    expect(onDuplicateClick).toHaveBeenCalledOnce();

    cleanup();
    renderActions({ onDuplicateClick, duplicateDisabled: true, duplicateDisabledReason: reason });

    const duplicateButton = screen.getByTestId(DUPLICATE_BUTTON_TEST_ID);
    expect((duplicateButton as HTMLButtonElement).disabled).toBe(true);
    expect(duplicateButton.parentElement?.getAttribute("aria-label")).toBe(reason);
  });

  it("labels the duplicate action while loading", () => {
    renderActions({ duplicateDisabled: true, duplicateLoading: true });

    const duplicateButton = screen.getByTestId(DUPLICATE_BUTTON_TEST_ID);
    expect(duplicateButton.parentElement?.getAttribute("aria-label")).toBe("Duplicating...");
  });
});
