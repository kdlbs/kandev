import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowSelectorRow } from "./workflow-selector-row";
import type { TaskCreateLaunchPreview } from "./task-create-dialog-launch-preview";

afterEach(cleanup);

const launchPreview: TaskCreateLaunchPreview = {
  stepId: "step-1",
  stepName: "In Progress",
  stepPrompt: "Run {{task_prompt}}",
};

function renderSelector(preview: TaskCreateLaunchPreview | null = launchPreview) {
  return render(
    <TooltipProvider>
      <WorkflowSelectorRow
        workflows={[{ id: "workflow-1", name: "Development" }]}
        snapshots={{}}
        selectedWorkflowId="workflow-1"
        onWorkflowChange={() => {}}
        agentProfiles={[]}
        launchPreview={preview}
      />
    </TooltipProvider>,
  );
}

describe("WorkflowSelectorRow launch destination", () => {
  it("shows the launch step outside and to the right of the selector", () => {
    renderSelector();

    const trigger = screen.getByTestId("workflow-selector-trigger");
    const row = screen.getByTestId("workflow-selector-row");
    const launchStep = screen.getByTestId("task-create-launch-step");

    expect(trigger.textContent).toContain("Development");
    expect(trigger.contains(launchStep)).toBe(false);
    expect(row.contains(trigger)).toBe(true);
    expect(row.contains(launchStep)).toBe(true);
    expect(launchStep.textContent).toBe("Start step: In Progress");
    expect(launchStep.className).toContain("ml-auto");
  });

  it("does not show a destination when the selected workflow has no preview", () => {
    renderSelector(null);

    expect(screen.queryByTestId("task-create-launch-step")).toBeNull();
  });
});
