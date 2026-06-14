import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskItem } from "./task-item";

const REVIEW_ICON_TEST_ID = "task-state-review";
const RUNNING_ICON_TEST_ID = "task-state-running";
const WAITING_FOR_INPUT_ICON_TEST_ID = "task-state-waiting-for-input";
const PENDING_PERMISSION_ICON_TEST_ID = "task-state-pending-permission";

afterEach(() => cleanup());

function renderTaskItem(props: Partial<ComponentProps<typeof TaskItem>> = {}) {
  return render(
    <StateProvider>
      <TaskItem title="Needs answer" state="REVIEW" {...props} />
    </StateProvider>,
  );
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

  it("shows a slower purple spinner while the workflow is scheduling", () => {
    renderTaskItem({ state: "SCHEDULING" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute("data-loading-phase")).toBe("preparing");
    expect(icon.classList.contains("text-purple-500")).toBe(true);
    expect(icon.classList.contains("animate-spin")).toBe(true);
    expect(icon.classList.contains("[animation-duration:2s]")).toBe(true);
  });

  it("shows a slower purple spinner while the session is starting before progress", () => {
    renderTaskItem({ state: "TODO", sessionState: "STARTING" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute("data-loading-phase")).toBe("preparing");
    expect(icon.classList.contains("text-purple-500")).toBe(true);
    expect(icon.classList.contains("animate-spin")).toBe(true);
    expect(icon.classList.contains("[animation-duration:2s]")).toBe(true);
  });

  it("keeps running tasks on the normal running spinner", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "RUNNING" });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute("data-loading-phase")).toBe("running");
    expect(icon.classList.contains("text-yellow-500")).toBe(true);
    expect(icon.classList.contains("animate-spin")).toBe(true);
    expect(icon.classList.contains("text-purple-500")).toBe(false);
  });

  it("keeps the review check for completed review tasks", () => {
    renderTaskItem({ sessionState: "COMPLETED" });

    expect(screen.queryByTestId(REVIEW_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).toBeNull();
  });
});
