import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { describe, expect, it, vi } from "vitest";
import { WorkflowCardHeaderActions } from "./workflow-card-header-actions";

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
          deleteDisabled={false}
          exportDisabled
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
});
