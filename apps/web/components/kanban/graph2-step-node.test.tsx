import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { ForegroundActivity, TaskPendingAction } from "@/lib/types/http";
import { Graph2StepNode } from "./graph2-step-node";

// The node renders inside the SPA router; stub it so the component mounts.
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

afterEach(() => {
  cleanup();
});

const STEP: WorkflowStep = { id: "step-1", title: "In Progress", color: "#888" };
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
    // §spec:task-level-indicator: idle foreground + live background work reads as
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

describe("Graph2StepNode — waiting-for-input variants (§spec:waiting-for-input-parity)", () => {
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
