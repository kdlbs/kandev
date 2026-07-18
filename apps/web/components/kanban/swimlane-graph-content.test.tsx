import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { ForegroundActivity } from "@/lib/types/http";
import { SwimlaneGraphContent } from "./swimlane-graph-content";

afterEach(() => {
  cleanup();
});

const STEPS: WorkflowStep[] = [{ id: "step-1", title: "In Progress", color: "#888" }];

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
      <SwimlaneGraphContent
        workflowId="wf-1"
        steps={STEPS}
        tasks={[makeTask(foregroundActivity)]}
        onPreviewTask={() => undefined}
      />
    </StateProvider>,
  );
}

describe("SwimlaneGraphContent — task-level background-running affordance", () => {
  it("shows the background spinner (IconLoader) for a background-running task chip, not the done check", () => {
    const { container } = renderSwimlane("background");
    // §spec:task-level-indicator: the swimlane task chip reflects the task-level
    // aggregate — background-running (IconLoader), never a done check for a task
    // still doing background work, even when the coarse state is COMPLETED.
    expect(container.querySelector(".tabler-icon-loader")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });

  it("shows the generating spinner (IconLoader2) when a session is generating", () => {
    const { container } = renderSwimlane("generating");
    expect(container.querySelector(".tabler-icon-loader-2")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
  });

  it("falls through to the coarse done check when no session is active", () => {
    const { container } = renderSwimlane(null);
    expect(container.querySelector(".tabler-icon-check")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });
});
