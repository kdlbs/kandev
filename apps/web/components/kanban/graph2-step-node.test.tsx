import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { ForegroundActivity, TaskPendingAction } from "@/lib/types/http";
import { Graph2StepNode } from "./graph2-step-node";

afterEach(() => {
  cleanup();
});

const STEP_TITLE = "In Progress";
const STEP: WorkflowStep = { id: "step-1", title: STEP_TITLE, color: "#888" };
const ICON_CHECK = ".tabler-icon-check";
const ICON_LOADER2 = ".tabler-icon-loader-2";

function makeTask(foregroundActivity?: ForegroundActivity | null): Task {
  return {
    id: "task-1",
    title: "A task",
    workflowStepId: "step-1",
    state: "COMPLETED",
    foregroundActivity,
  } as Task;
}

function renderCurrentNode(foregroundActivity?: ForegroundActivity | null) {
  return render(
    <StateProvider>
      <Graph2StepNode
        step={STEP}
        phase="current"
        task={makeTask(foregroundActivity)}
        hasPrev={false}
        hasNext={false}
        onMoveTask={() => undefined}
        onOpenTask={() => undefined}
      />
    </StateProvider>,
  );
}

describe("Graph2StepNode — task-level background-running affordance", () => {
  it("shows the background spinner (IconLoader) for a background-running task, not the done check", () => {
    const { container } = renderCurrentNode("background");
    // idle foreground + live background work reads as
    // background-running (segmented IconLoader), never the done check — even when
    // the coarse task state is COMPLETED.
    expect(container.querySelector(".tabler-icon-loader")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    // Distinct by SHAPE from the generating spinner (IconLoader2), not hue alone.
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the generating spinner (IconLoader2) when any session is generating", () => {
    const { container } = renderCurrentNode("generating");
    expect(container.querySelector(ICON_LOADER2)).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
  });

  it("falls through to the coarse done check when no session is active", () => {
    const { container } = renderCurrentNode(null);
    expect(container.querySelector(ICON_CHECK)).not.toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});

describe("Graph2StepNode — auto-start-failed marker", () => {
  function renderNodeWithTask(task: Task) {
    return render(
      <StateProvider>
        <TooltipProvider>
          <Graph2StepNode
            step={STEP}
            phase="current"
            task={task}
            hasPrev={false}
            hasNext={false}
            onMoveTask={() => undefined}
            onOpenTask={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>,
    );
  }

  it("shows the auto-start-failed triangle for a non-terminal task marked auto_start_failed", () => {
    const task = {
      id: "task-1",
      title: "A task",
      workflowStepId: "step-1",
      state: "IN_PROGRESS",
      autoStartFailed: true,
    } as Task;
    const { container } = renderNodeWithTask(task);
    expect(container.querySelector('[data-testid="task-state-auto-start-failed"]')).not.toBeNull();
  });

  it("does not show the auto-start-failed triangle when the marker is absent", () => {
    const task = {
      id: "task-1",
      title: "A task",
      workflowStepId: "step-1",
      state: "IN_PROGRESS",
      autoStartFailed: false,
    } as Task;
    const { container } = renderNodeWithTask(task);
    expect(container.querySelector('[data-testid="task-state-auto-start-failed"]')).toBeNull();
  });
});

describe("Graph2StepNode — waiting-for-input variants", () => {
  function renderWaitingNode(pendingAction: TaskPendingAction) {
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
        <Graph2StepNode
          step={STEP}
          phase="current"
          task={task}
          hasPrev={false}
          hasNext={false}
          onMoveTask={() => undefined}
          onOpenTask={() => undefined}
        />
      </StateProvider>,
    );
  }

  it("shows the message-question for a pending clarification, distinct from done and running", () => {
    const { container } = renderWaitingNode("clarification");
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });

  it("shows the shield-question for a pending permission, distinct from done and running", () => {
    const { container } = renderWaitingNode("permission");
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
    expect(container.querySelector(ICON_CHECK)).toBeNull();
    expect(container.querySelector(ICON_LOADER2)).toBeNull();
  });
});

describe("Graph2StepNode — hidden destination disclosure", () => {
  function renderNextMove(nextStepHidden: boolean) {
    render(
      <StateProvider>
        <TooltipProvider delayDuration={0}>
          <Graph2StepNode
            step={STEP}
            phase="current"
            task={makeTask()}
            hasPrev={false}
            hasNext
            nextStepId="step-done"
            nextStepTitle="Done"
            nextStepHidden={nextStepHidden}
            onMoveTask={() => undefined}
            onOpenTask={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>,
    );
    const currentStep = screen.getByRole("button", { name: STEP_TITLE });
    fireEvent.mouseEnter(currentStep.parentElement!);
    return screen.getByRole("button", { name: "Move to Done" });
  }

  it("shows the destination tooltip only when the destination is hidden", async () => {
    const hiddenTargetButton = renderNextMove(true);
    fireEvent.focus(hiddenTargetButton);
    await waitFor(() => expect(screen.getByRole("tooltip").textContent).toBe("Move to Done"));

    cleanup();
    const visibleTargetButton = renderNextMove(false);
    fireEvent.focus(visibleTargetButton);
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("exposes move controls when the current node receives keyboard focus", () => {
    render(
      <StateProvider>
        <TooltipProvider>
          <Graph2StepNode
            step={STEP}
            phase="current"
            task={makeTask()}
            hasPrev={false}
            hasNext
            nextStepId="step-done"
            nextStepTitle="Done"
            nextStepHidden
            onMoveTask={() => undefined}
            onOpenTask={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    fireEvent.focus(screen.getByRole("button", { name: STEP_TITLE }));

    expect(screen.getByRole("button", { name: "Move to Done" })).not.toBeNull();
  });
});

describe("Graph2StepNode — pill click routes through onOpenTask", () => {
  it("calls onOpenTask and does not navigate directly on its own", () => {
    const onOpenTask = vi.fn();
    render(
      <StateProvider>
        <Graph2StepNode
          step={STEP}
          phase="current"
          task={makeTask()}
          hasPrev={false}
          hasNext={false}
          onMoveTask={() => undefined}
          onOpenTask={onOpenTask}
        />
      </StateProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: STEP_TITLE }));

    expect(onOpenTask).toHaveBeenCalledWith(makeTask());
  });
});
