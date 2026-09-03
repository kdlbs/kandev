import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskPreviewPanel } from "./task-preview-panel";
import type { WorkflowStepperStep } from "./task/workflow-step-disclosure";
import type { Task } from "./kanban-card";

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => "workspace-1",
}));

vi.mock("./task/preview-session-tabs", () => ({
  PreviewSessionTabs: () => <div data-testid="preview-session-tabs" />,
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => false,
}));

afterEach(cleanup);

const TASK: Task = {
  id: "task-1",
  title: "Fix the login bug",
} as Task;

const WORKFLOW_ID = "workflow-1";
const STEP_A_ID = "a";
const STEPPER_TEST_ID = "workflow-stepper-minimal";

const STEPS: WorkflowStepperStep[] = [
  { id: "a", name: "Spec", color: "#111", position: 0 },
  { id: "b", name: "Work", color: "#222", position: 1, allow_manual_move: true },
];

describe("TaskPreviewPanel step indicator", () => {
  it("shows no step indicator when there are no resolved workflow steps", () => {
    render(<TaskPreviewPanel task={TASK} onClose={vi.fn()} workflowSteps={[]} />);

    expect(screen.getByText("Fix the login bug")).toBeTruthy();
    expect(screen.queryByTestId(STEPPER_TEST_ID)).toBeNull();
  });

  it("shows no step indicator when there is no selected task", () => {
    render(
      <TaskPreviewPanel
        task={null}
        onClose={vi.fn()}
        workflowSteps={STEPS}
        currentStepId={STEP_A_ID}
        taskWorkflowId={WORKFLOW_ID}
        onMoveStep={vi.fn()}
      />,
    );

    expect(screen.queryByTestId(STEPPER_TEST_ID)).toBeNull();
  });

  it("shows the current step and total between the title and the panel controls", () => {
    render(
      <TaskPreviewPanel
        task={TASK}
        onClose={vi.fn()}
        workflowSteps={STEPS}
        currentStepId={STEP_A_ID}
        taskWorkflowId={WORKFLOW_ID}
        onMoveStep={vi.fn()}
      />,
    );

    expect(screen.getByTestId(STEPPER_TEST_ID)).toBeTruthy();
    expect(screen.getByText("1/2")).toBeTruthy();
  });

  it("issues a move for an eligible target selected from the disclosure", async () => {
    const onMoveStep = vi.fn().mockResolvedValue(true);
    render(
      <TaskPreviewPanel
        task={TASK}
        onClose={vi.fn()}
        workflowSteps={STEPS}
        currentStepId={STEP_A_ID}
        taskWorkflowId={WORKFLOW_ID}
        onMoveStep={onMoveStep}
      />,
    );

    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    fireEvent.click(screen.getByTestId("workflow-step-disclosure-move-b"));

    expect(onMoveStep).toHaveBeenCalledWith("b");
  });

  it("notifies the caller when the disclosure opens and closes", () => {
    const onDisclosureOpenChange = vi.fn();
    render(
      <TaskPreviewPanel
        task={TASK}
        onClose={vi.fn()}
        workflowSteps={STEPS}
        currentStepId={STEP_A_ID}
        taskWorkflowId={WORKFLOW_ID}
        onMoveStep={vi.fn()}
        onDisclosureOpenChange={onDisclosureOpenChange}
      />,
    );

    expect(onDisclosureOpenChange).toHaveBeenCalledWith(false);
    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    expect(onDisclosureOpenChange).toHaveBeenCalledWith(true);
  });

  it("renders the move-failure message below the header, not inside it", () => {
    render(
      <TaskPreviewPanel
        task={TASK}
        onClose={vi.fn()}
        workflowSteps={STEPS}
        currentStepId={STEP_A_ID}
        taskWorkflowId={WORKFLOW_ID}
        onMoveStep={vi.fn()}
        moveError={new Error("boom")}
      />,
    );

    const banner = screen.getByTestId("task-move-error-banner");
    expect(banner).toBeTruthy();
    const headerRow = screen.getByText("Fix the login bug").closest(".border-b");
    expect(headerRow?.contains(banner)).toBe(false);
  });

  it("shows no move-failure message when there is none", () => {
    render(<TaskPreviewPanel task={TASK} onClose={vi.fn()} workflowSteps={[]} />);
    expect(screen.queryByTestId("task-move-error-banner")).toBeNull();
  });
});
