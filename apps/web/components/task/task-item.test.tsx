import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskItem } from "./task-item";
import { TooltipProvider } from "@kandev/ui/tooltip";

const REVIEW_ICON_TEST_ID = "task-state-review";
const RUNNING_ICON_TEST_ID = "task-state-running";
const WAITING_FOR_INPUT_ICON_TEST_ID = "task-state-waiting-for-input";
const PENDING_PERMISSION_ICON_TEST_ID = "task-state-pending-permission";
const AGENT_ERROR_ICON_TEST_ID = "task-agent-error-icon";
const PREPARING_PHASE = "preparing";
const PREPARING_SPINNER_CLASS = "text-muted-foreground/40";
const SPIN_CLASS = "animate-spin";
const SLOW_SPIN_CLASS = "[animation-duration:2s]";

afterEach(() => cleanup());

function renderTaskItem(props: Partial<ComponentProps<typeof TaskItem>> = {}) {
  return render(
    <StateProvider>
      <TooltipProvider>
        <TaskItem title="Needs answer" state="REVIEW" {...props} />
      </TooltipProvider>
    </StateProvider>,
  );
}

function expectPreparingSpinner(): void {
  const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
  expect(icon.getAttribute("data-loading-phase")).toBe(PREPARING_PHASE);
  expect(icon.classList.contains(PREPARING_SPINNER_CLASS)).toBe(true);
  expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
  expect(icon.classList.contains(SLOW_SPIN_CLASS)).toBe(true);
}

describe("TaskItem status icon", () => {
  it("keeps the review check when the session is idle after a turn", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT" });

    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("shows question icon when a clarification is pending", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT", hasPendingClarification: true });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).toBeNull();
  });

  it("shows question icon when task state is waiting for input", () => {
    renderTaskItem({ state: "WAITING_FOR_INPUT", hasPendingClarification: false });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).toBeNull();
  });

  it("shows alert icon when a permission request is pending", () => {
    renderTaskItem({ sessionState: "WAITING_FOR_INPUT", hasPendingPermission: true });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("prefers clarification icon over permission icon when both are pending", () => {
    renderTaskItem({
      sessionState: "WAITING_FOR_INPUT",
      hasPendingClarification: true,
      hasPendingPermission: true,
    });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).toBeNull();
  });

  it("shows a slower muted spinner while the workflow is scheduling", () => {
    renderTaskItem({ state: "SCHEDULING" });

    expectPreparingSpinner();
  });

  it("shows a slower muted spinner while the session is starting before progress", () => {
    renderTaskItem({ state: "TODO", sessionState: "STARTING" });

    expectPreparingSpinner();
  });

  it("shows a slower muted spinner for in-progress tasks while the session is starting", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "STARTING" });

    expectPreparingSpinner();
  });

  it("does not show a spinner for a created task waiting for manual start", () => {
    renderTaskItem({ state: "CREATED" });

    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-state-backlog")).not.toBeNull();
  });

  it("does not show a spinner for a created task with a prepared CREATED session", () => {
    renderTaskItem({ state: "CREATED", sessionState: "CREATED" });

    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-state-backlog")).not.toBeNull();
  });

  it("shows a running spinner while an in-progress session is created but not started", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "CREATED" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute("data-loading-phase")).toBe("running");
    expect(icon.classList.contains("text-yellow-500")).toBe(true);
    expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
  });

  it("does not show preparing spinner for a review task whose session transiently hits STARTING", () => {
    renderTaskItem({ state: "REVIEW", sessionState: "STARTING" });

    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
  });

  it("keeps running tasks on the normal running spinner", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "RUNNING" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute("data-loading-phase")).toBe("running");
    expect(icon.classList.contains("text-yellow-500")).toBe(true);
    expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
    expect(icon.classList.contains(PREPARING_SPINNER_CLASS)).toBe(false);
  });

  it("keeps the review check for completed review tasks", () => {
    renderTaskItem({ sessionState: "COMPLETED" });

    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });

  it("shows a separate agent error icon when the task has retained error details", () => {
    renderTaskItem({ agentErrorMessage: "peer disconnected before response" });

    const errorIcon = screen.getByTestId(AGENT_ERROR_ICON_TEST_ID);
    expect(errorIcon).not.toBeNull();
    expect(errorIcon.classList.contains("cursor-help")).toBe(true);
    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).not.toBeNull();
  });
});

describe("TaskItem actions", () => {
  it("announces the task menu state", () => {
    renderTaskItem({ menuOpen: true });

    const actions = screen.getByRole("button", { name: "Task actions" });
    expect(actions.getAttribute("aria-haspopup")).toBe("menu");
    expect(actions.getAttribute("aria-expanded")).toBe("true");
  });

  it("does not announce a closed menu as expanded while deleting", () => {
    renderTaskItem({ isDeleting: true });

    expect(screen.getByRole("button", { name: "Task actions" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });
});
