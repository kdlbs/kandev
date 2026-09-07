import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowSelectorRow } from "./workflow-selector-row";
import type { TaskCreateLaunchPreview } from "./task-create-dialog-launch-preview";

const touchState = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));

afterEach(() => {
  touchState.enabled = false;
  cleanup();
});

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
  // @covers AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1
  it("shows the launch destination arrow and explanation after the selector", async () => {
    renderSelector();

    const trigger = screen.getByTestId("workflow-selector-trigger");
    const row = screen.getByTestId("workflow-selector-row");
    const launchStep = screen.getByTestId("task-create-launch-step");

    expect(trigger.textContent).toContain("Development");
    expect(trigger.contains(launchStep)).toBe(false);
    expect(row.contains(trigger)).toBe(true);
    expect(row.contains(launchStep)).toBe(true);
    expect(trigger.className).not.toContain("flex-1");
    expect(launchStep.textContent).toBe("In Progress");
    expect(launchStep.className).not.toContain("ml-auto");

    const launchStepInfo = screen.getByTestId("task-create-launch-step-info");
    expect(trigger.contains(launchStepInfo)).toBe(false);
    expect(launchStepInfo.getAttribute("aria-label")).toBe("Learn about the task start step");
    expect(screen.getByTestId("task-create-launch-step-arrow")).toBeTruthy();
    expect(Array.from(row.children).indexOf(launchStepInfo)).toBeLessThan(
      Array.from(row.children).indexOf(launchStep),
    );

    fireEvent.focus(launchStepInfo);
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      "The task starts in this workflow step. With a task description, an auto-start step can take priority over the configured Start step.",
    );
  });

  it("opens the explanation in a drawer for coarse pointers", () => {
    touchState.enabled = true;
    renderSelector();

    const launchStepInfo = screen.getByTestId("task-create-launch-step-info");
    expect(launchStepInfo.getAttribute("aria-haspopup")).toBe("dialog");
    expect(launchStepInfo.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(launchStepInfo);

    expect(launchStepInfo.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId("task-create-launch-step-help-drawer").textContent).toContain(
      "The task starts in this workflow step.",
    );
  });

  it("does not show a destination when the selected workflow has no preview", () => {
    renderSelector(null);

    expect(screen.queryByTestId("task-create-launch-step")).toBeNull();
  });
});
