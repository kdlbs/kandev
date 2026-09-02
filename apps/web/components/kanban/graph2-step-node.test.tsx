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
        onPreviewTask={() => undefined}
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
            onPreviewTask={() => undefined}
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
          onPreviewTask={() => undefined}
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

describe("Graph2StepNode — collapsed step markers (AC-UI-PIPELINE-ROW-001)", () => {
  const PAST_STEP: WorkflowStep = { id: "step-0", title: "Triage", color: "#888" };
  const FUTURE_STEP: WorkflowStep = { id: "step-2", title: "Review", color: "#888" };

  function renderCollapsed(phase: "past" | "future", onMoveTask = vi.fn()) {
    const step = phase === "past" ? PAST_STEP : FUTURE_STEP;
    const result = render(
      <StateProvider>
        <TooltipProvider delayDuration={0}>
          <Graph2StepNode
            step={step}
            phase={phase}
            task={makeTask()}
            hasPrev={false}
            hasNext={false}
            onMoveTask={onMoveTask}
            onPreviewTask={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>,
    );
    return { step, onMoveTask, unmount: result.unmount };
  }

  function getMarker(phase: "past" | "future") {
    return screen.getByTestId(`graph2-step-node-collapsed-${phase}`);
  }

  it("renders a completed step as a collapsed marker with no visible text label", () => {
    const { step } = renderCollapsed("past");
    expect(getMarker("past").textContent).not.toBe(step.title);
    expect(screen.queryByText(step.title)).toBeNull();
  });

  it("renders a not-yet-reached step as a collapsed marker with no visible text label", () => {
    const { step } = renderCollapsed("future");
    expect(getMarker("future").textContent).not.toBe(step.title);
    expect(screen.queryByText(step.title)).toBeNull();
  });

  it("renders completed and not-yet-reached markers with visually distinct styling", () => {
    const { unmount } = renderCollapsed("past");
    const pastClassName = getMarker("past").className;
    unmount();

    renderCollapsed("future");
    const futureClassName = getMarker("future").className;

    expect(pastClassName).not.toBe(futureClassName);
  });

  it("never renders a collapsed marker narrower than the 12px floor", () => {
    renderCollapsed("past");
    const marker = getMarker("past");
    // jsdom performs no layout, so the 12px (w-3/h-3, 0.75rem @ 16px root) floor
    // is verified via the sizing classes rather than a computed pixel value.
    expect(marker.className).toMatch(/\bw-3\b/);
    expect(marker.className).toMatch(/\bh-3\b/);
  });

  it("is not individually focusable, so tab stop count does not grow with step count", () => {
    renderCollapsed("past");
    const marker = getMarker("past");
    expect(marker.tagName).not.toBe("BUTTON");
    expect(marker.getAttribute("role")).not.toBe("button");
    expect(marker.hasAttribute("tabindex")).toBe(false);
  });

  it("is not a click target for moving the task", () => {
    const { onMoveTask } = renderCollapsed("past");
    fireEvent.click(getMarker("past"));
    expect(onMoveTask).not.toHaveBeenCalled();
  });

  it("discloses the step title in a hover tooltip on a fine pointer", async () => {
    const { step } = renderCollapsed("future");
    fireEvent.pointerMove(getMarker("future"), { pointerType: "mouse" });
    await waitFor(() => expect(screen.getByRole("tooltip").textContent).toBe(step.title));
  });
});

describe("Graph2StepNode — move-control destination disclosure", () => {
  function renderNextMove() {
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
            onMoveTask={() => undefined}
            onPreviewTask={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>,
    );
    const currentStep = screen.getByRole("button", { name: STEP_TITLE });
    fireEvent.mouseEnter(currentStep.parentElement!);
    return screen.getByRole("button", { name: "Move to Done" });
  }

  it("AC-UI-PIPELINE-ROW-001.7: always shows the destination tooltip on focus", async () => {
    const targetButton = renderNextMove();
    fireEvent.focus(targetButton);
    await waitFor(() => expect(screen.getByRole("tooltip").textContent).toBe("Move to Done"));
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
            onMoveTask={() => undefined}
            onPreviewTask={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    fireEvent.focus(screen.getByRole("button", { name: STEP_TITLE }));

    expect(screen.getByRole("button", { name: "Move to Done" })).not.toBeNull();
  });
});
