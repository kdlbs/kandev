import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import { TaskItem } from "./task-item";
import { TooltipProvider } from "@kandev/ui/tooltip";

const BACKGROUND_ICON_TEST_ID = "task-state-background-running";
const WAITING_FOR_INPUT_ICON_TEST_ID = "task-state-waiting-for-input";
const PENDING_PERMISSION_ICON_TEST_ID = "task-state-pending-permission";
const TURN_FINISHED_ICON_TEST_ID = "task-state-turn-finished";
const RUNNING_ICON_TEST_ID = "task-state-running";
const DATA_LOADING_PHASE = "data-loading-phase";
const RUNNING_PHASE = "running";
const YELLOW_SPINNER_CLASS = "text-yellow-500";
const VIOLET_SPINNER_CLASS = "text-violet-500";
const SPIN_CLASS = "animate-spin";

afterEach(() => cleanup());

function renderTaskItem(
  props: Partial<ComponentProps<typeof TaskItem>> = {},
  initialState: HydrationState = {},
) {
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <TaskItem title="Needs answer" state="REVIEW" {...props} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TaskItem parked-on-background-work indicator", () => {
  // AC-23: a settled task whose only remaining life is a positively-sampled
  // background process renders the same tooltip-carrying affordance as a live
  // foregroundActivity=background aggregate.
  it("shows the background-running affordance when parked on background work", () => {
    renderTaskItem({
      state: "REVIEW",
      sessionState: "COMPLETED",
      foregroundActivity: null,
      parkedOnBackgroundWork: true,
    });

    const icon = screen.getByTestId(BACKGROUND_ICON_TEST_ID);
    expect(icon.classList.contains(VIOLET_SPINNER_CLASS)).toBe(true);
    expect(icon.classList.contains(SPIN_CLASS)).toBe(true);
    expect(screen.getByLabelText("Background work is running")).not.toBeNull();
    expect(screen.queryByTestId(TURN_FINISHED_ICON_TEST_ID)).toBeNull();
  });

  // AC-34: pending-input (clarification/permission) and any live
  // foregroundActivity outrank the parked affordance; it never overrides them.
  it("shows pending clarification instead of parked-on-background-work", () => {
    renderTaskItem({
      state: "WAITING_FOR_INPUT",
      sessionState: "WAITING_FOR_INPUT",
      parkedOnBackgroundWork: true,
      hasPendingClarification: true,
    });

    expect(screen.queryByTestId(WAITING_FOR_INPUT_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();
  });

  it("shows pending permission instead of parked-on-background-work", () => {
    renderTaskItem({
      state: "WAITING_FOR_INPUT",
      sessionState: "RUNNING",
      parkedOnBackgroundWork: true,
      hasPendingPermission: true,
    });

    expect(screen.queryByTestId(PENDING_PERMISSION_ICON_TEST_ID)).not.toBeNull();
    expect(screen.queryByTestId(BACKGROUND_ICON_TEST_ID)).toBeNull();
  });

  it("shows the generating spinner instead of parked-on-background-work when both are reported", () => {
    // Stale/racing signals: a live generating aggregate must still win over a
    // parked flag that has not cleared yet.
    renderTaskItem({
      state: "IN_PROGRESS",
      sessionState: "RUNNING",
      foregroundActivity: "generating",
      parkedOnBackgroundWork: true,
    });

    const icon = screen.getByTestId(RUNNING_ICON_TEST_ID);
    expect(icon.getAttribute(DATA_LOADING_PHASE)).toBe(RUNNING_PHASE);
    expect(icon.classList.contains(YELLOW_SPINNER_CLASS)).toBe(true);
  });
});
