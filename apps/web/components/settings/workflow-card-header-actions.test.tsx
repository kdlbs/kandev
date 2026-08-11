import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkflowCardHeaderActions } from "./workflow-card-header-actions";

afterEach(cleanup);

describe("WorkflowCardHeaderActions", () => {
  it("surfaces failures when deleting a temporary workflow", async () => {
    const failure = new Error("cleanup failed");
    const toast = vi.fn();

    render(
      <TooltipProvider>
        <WorkflowCardHeaderActions
          workflowId="temp-workflow-1"
          setExportYaml={vi.fn()}
          setExportOpen={vi.fn()}
          toast={toast}
          onDeleteClick={vi.fn().mockRejectedValue(failure)}
          onDuplicateClick={vi.fn().mockResolvedValue(undefined)}
          deleteDisabled={false}
          exportDisabled
          duplicateDisabled
          duplicateLoading={false}
          readOnly={false}
        />
      </TooltipProvider>,
    );

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
    render(
      <TooltipProvider>
        <WorkflowCardHeaderActions
          workflowId="workflow-1"
          setExportYaml={vi.fn()}
          setExportOpen={vi.fn()}
          toast={vi.fn()}
          onDeleteClick={vi.fn().mockResolvedValue(undefined)}
          onDuplicateClick={vi.fn().mockResolvedValue(undefined)}
          deleteDisabled={false}
          exportDisabled={false}
          duplicateDisabled={false}
          duplicateLoading={false}
          readOnly={false}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("duplicate-workflow-button")).toBeTruthy();
  });

  it("invokes duplication and exposes the save-first disabled reason", () => {
    const onDuplicateClick = vi.fn().mockResolvedValue(undefined);
    const reason = "Save the workflow before duplicating it.";

    render(
      <TooltipProvider>
        <WorkflowCardHeaderActions
          workflowId="workflow-1"
          setExportYaml={vi.fn()}
          setExportOpen={vi.fn()}
          toast={vi.fn()}
          onDeleteClick={vi.fn().mockResolvedValue(undefined)}
          onDuplicateClick={onDuplicateClick}
          deleteDisabled={false}
          exportDisabled={false}
          duplicateDisabled={false}
          duplicateLoading={false}
          readOnly={false}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("duplicate-workflow-button"));
    expect(onDuplicateClick).toHaveBeenCalledOnce();

    cleanup();
    render(
      <TooltipProvider>
        <WorkflowCardHeaderActions
          workflowId="workflow-1"
          setExportYaml={vi.fn()}
          setExportOpen={vi.fn()}
          toast={vi.fn()}
          onDeleteClick={vi.fn().mockResolvedValue(undefined)}
          onDuplicateClick={onDuplicateClick}
          deleteDisabled={false}
          exportDisabled={false}
          duplicateDisabled
          duplicateDisabledReason={reason}
          duplicateLoading={false}
          readOnly={false}
        />
      </TooltipProvider>,
    );

    const duplicateButton = screen.getByTestId("duplicate-workflow-button");
    expect((duplicateButton as HTMLButtonElement).disabled).toBe(true);
    expect(duplicateButton.parentElement?.getAttribute("aria-label")).toBe(reason);
  });
});
