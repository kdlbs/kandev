import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { TaskItem } from "./task-item";

const READY_ICON_TEST_ID = "task-state-ready";
const RUNNING_ICON_TEST_ID = "task-state-running";
const WAITING_FOR_INPUT_ICON_TEST_ID = "task-state-waiting-for-input";
const PENDING_PERMISSION_ICON_TEST_ID = "task-state-pending-permission";

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

describe("TaskItem ready status icon", () => {
  it("shows the green ready check when the session is idle", () => {
    renderTaskItem({ state: "IN_PROGRESS", sessionState: "IDLE" });

    const icon = screen.getByTestId(READY_ICON_TEST_ID);
    expect(icon.classList.contains("text-green-500")).toBe(true);
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-state-backlog")).toBeNull();
  });

  it("keeps pending clarification ahead of the idle ready state", () => {
    renderTaskItem({ sessionState: "IDLE", hasPendingClarification: true });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(READY_ICON_TEST_ID)).toBeNull();
  });

  it("keeps pending permission ahead of the idle ready state", () => {
    renderTaskItem({ sessionState: "IDLE", hasPendingPermission: true });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(READY_ICON_TEST_ID)).toBeNull();
  });
});
