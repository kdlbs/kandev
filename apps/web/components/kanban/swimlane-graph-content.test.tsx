import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { ForegroundActivity, TaskPendingAction } from "@/lib/types/http";
import { SwimlaneGraphContent } from "./swimlane-graph-content";

afterEach(() => {
  cleanup();
});

const STEPS: WorkflowStep[] = [{ id: "step-1", title: "In Progress", color: "#888" }];
const ICON_CHECK = ".tabler-icon-check";
const ICON_LOADER2 = ".tabler-icon-loader-2";
const BG_TESTID = '[data-testid="task-state-background-running"]';

function makeTask(foregroundActivity?: ForegroundActivity | null): Task {
  return {
    id: "task-1",
    title: "A task",
    workflowStepId: "step-1",
    state: "COMPLETED",
    foregroundActivity,
  } as Task;
}

function renderSwimlane(foregroundActivity?: ForegroundActivity | null) {
  return render(
    <StateProvider>
      <TooltipProvider>
        <SwimlaneGraphContent
          workflowId="wf-1"
          steps={STEPS}
          tasks={[makeTask(foregroundActivity)]}
          onPreviewTask={() => undefined}
        />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("SwimlaneGraphContent — task-level background-running affordance", () => {
  it("shows the background-running affordance for a background-running task chip, not the done check", () => {
    const { container } = renderSwimlane("background");
    // the swimlane task chip reflects the task-level
    // aggregate — background-running (shared BackgroundWorkTaskIcon), never a done
    // check for a task still doing background work, even when the coarse state is
    // COMPLETED.
    expect(container.querySelector(BG_TESTID)).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the generating spinner (IconLoader2) when a session is generating", () => {
    const { container } = renderSwimlane("generating");
    expect(container.querySelector(ICON_LOADER2)).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
  });

  it("falls through to the coarse done check when no session is active", () => {
    const { container } = renderSwimlane(null);
    expect(container.querySelector(ICON_CHECK)).not.toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});

describe("SwimlaneGraphContent — waiting-for-input variants", () => {
  function renderWaiting(pendingAction: TaskPendingAction) {
    const task = {
      id: "task-1",
      title: "A task",
      workflowStepId: "step-1",
      state: "WAITING_FOR_INPUT",
      primarySessionId: "session-1",
      primarySessionState: "WAITING_FOR_INPUT",
      primarySessionPendingAction: pendingAction,
    } as Task;
    return render(
      <StateProvider>
        <SwimlaneGraphContent
          workflowId="wf-1"
          steps={STEPS}
          tasks={[task]}
          onPreviewTask={() => undefined}
        />
      </StateProvider>,
    );
  }

  it("shows the message-question for a pending clarification, distinct from done and running", () => {
    const { container } = renderWaiting("clarification");
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the shield-question for a pending permission, distinct from done and running", () => {
    const { container } = renderWaiting("permission");
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});
