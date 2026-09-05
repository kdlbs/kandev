import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskPreviewPanel } from "./task-preview-panel";
import type { WorkflowStepperStep } from "./task/workflow-step-disclosure";
import type { Task } from "./kanban-card";

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

const OTHER_TASK: Task = {
  id: "task-2",
  title: "Write the release notes",
} as Task;

const WORKFLOW_ID = "workflow-1";
const STEP_A_ID = "a";
const STEPPER_TEST_ID = "workflow-stepper-minimal";
const DISCLOSURE_TEST_ID = "workflow-step-disclosure";

const STEPS: WorkflowStepperStep[] = [
  { id: "a", name: "Spec", color: "#111", position: 0 },
  { id: "b", name: "Work", color: "#222", position: 1, allow_manual_move: true },
];

function withProviders(ui: ReactNode) {
  return (
    <ToastProvider>
      <StateProvider>{ui}</StateProvider>
    </ToastProvider>
  );
}

function renderPanel(overrides: Partial<ComponentProps<typeof TaskPreviewPanel>> = {}) {
  return render(
    withProviders(
      <TaskPreviewPanel
        task={TASK}
        onClose={vi.fn()}
        workflowSteps={STEPS}
        currentStepId={STEP_A_ID}
        taskWorkflowId={WORKFLOW_ID}
        onMoveStep={vi.fn()}
        {...overrides}
      />,
    ),
  );
}

describe("TaskPreviewPanel step indicator", () => {
  it("shows no step indicator when there are no resolved workflow steps", () => {
    renderPanel({ workflowSteps: [] });

    expect(screen.getByText("Fix the login bug")).toBeTruthy();
    expect(screen.queryByTestId(STEPPER_TEST_ID)).toBeNull();
  });

  it("shows no step indicator when there is no selected task", () => {
    renderPanel({ task: null });

    expect(screen.queryByTestId(STEPPER_TEST_ID)).toBeNull();
  });

  it("shows the current step and total between the title and the panel controls", () => {
    renderPanel();

    expect(screen.getByTestId(STEPPER_TEST_ID)).toBeTruthy();
    expect(screen.getByText("1/2")).toBeTruthy();
  });

  it("issues a move for an eligible target and closes the disclosure on success", async () => {
    const onMoveStep = vi.fn().mockResolvedValue(true);
    renderPanel({ onMoveStep });

    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    await act(async () => {
      fireEvent.click(screen.getByTestId("workflow-step-disclosure-move-b"));
      await Promise.resolve();
    });

    expect(onMoveStep).toHaveBeenCalledWith("b");
    expect(screen.queryByTestId(DISCLOSURE_TEST_ID)).toBeNull();
  });

  it("notifies the caller when the disclosure opens and closes", () => {
    const onDisclosureOpenChange = vi.fn();
    renderPanel({ onDisclosureOpenChange });

    expect(onDisclosureOpenChange).toHaveBeenCalledWith(false);
    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    expect(onDisclosureOpenChange).toHaveBeenCalledWith(true);
  });

  it("renders the move-failure message below the header, not inside it", () => {
    renderPanel({ moveError: new Error("boom") });

    const banner = screen.getByTestId("task-move-error-banner");
    expect(banner).toBeTruthy();
    const headerRow = screen.getByText("Fix the login bug").closest(".border-b");
    expect(headerRow?.contains(banner)).toBe(false);
  });

  it("shows no move-failure message when there is none", () => {
    renderPanel({ workflowSteps: [] });
    expect(screen.queryByTestId("task-move-error-banner")).toBeNull();
  });

  it("closes the disclosure when the previewed task changes while it is open", () => {
    const { rerender } = renderPanel();

    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    expect(screen.getByTestId(DISCLOSURE_TEST_ID)).toBeTruthy();

    rerender(
      withProviders(
        <TaskPreviewPanel
          task={OTHER_TASK}
          onClose={vi.fn()}
          workflowSteps={STEPS}
          currentStepId={STEP_A_ID}
          taskWorkflowId={WORKFLOW_ID}
          onMoveStep={vi.fn()}
        />,
      ),
    );

    expect(screen.queryByTestId(DISCLOSURE_TEST_ID)).toBeNull();
    expect(screen.getByText(OTHER_TASK.title)).toBeTruthy();
  });

  it("does not let a move started before a task switch close the new task's disclosure", async () => {
    let resolveMove!: (moved: boolean) => void;
    const onMoveStepForA = vi.fn(() => new Promise<boolean>((resolve) => (resolveMove = resolve)));
    const { rerender } = renderPanel({ onMoveStep: onMoveStepForA });

    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    fireEvent.click(screen.getByTestId("workflow-step-disclosure-move-b"));
    expect(onMoveStepForA).toHaveBeenCalledWith("b");

    rerender(
      withProviders(
        <TaskPreviewPanel
          task={OTHER_TASK}
          onClose={vi.fn()}
          workflowSteps={STEPS}
          currentStepId={STEP_A_ID}
          taskWorkflowId={WORKFLOW_ID}
          onMoveStep={vi.fn()}
        />,
      ),
    );

    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    expect(screen.getByTestId(DISCLOSURE_TEST_ID)).toBeTruthy();

    await act(async () => {
      resolveMove(true);
      await Promise.resolve();
    });

    expect(screen.getByTestId(DISCLOSURE_TEST_ID)).toBeTruthy();
  });

  it("shows the archived badge and offers no move control for an archived task", () => {
    const onMoveStep = vi.fn();
    renderPanel({ isArchived: true, onMoveStep });

    expect(screen.queryByTestId("workflow-step-disclosure-move-b")).toBeNull();
    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    expect(screen.queryByTestId(DISCLOSURE_TEST_ID)).toBeNull();
    expect(onMoveStep).not.toHaveBeenCalled();
  });
});

describe("TaskPreviewPanel step indicator lifecycle", () => {
  it("reports the disclosure closed when the stepper unmounts", () => {
    const onDisclosureOpenChange = vi.fn();
    const { rerender } = renderPanel({ onDisclosureOpenChange });

    fireEvent.mouseEnter(screen.getByTestId(STEPPER_TEST_ID));
    expect(onDisclosureOpenChange).toHaveBeenLastCalledWith(true);

    rerender(withProviders(<TaskPreviewPanel task={TASK} onClose={vi.fn()} workflowSteps={[]} />));

    expect(onDisclosureOpenChange).toHaveBeenLastCalledWith(false);
  });
});
