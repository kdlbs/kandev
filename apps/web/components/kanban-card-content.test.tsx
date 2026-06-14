import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DraggableAttributes, DraggableSyntheticListeners } from "@dnd-kit/core";
import { StateProvider } from "@/components/state-provider";
import { KanbanCardShell } from "./kanban-card-content";
import type { Task } from "./kanban-card";

afterEach(() => cleanup());

const dragProps = {
  attributes: {} as DraggableAttributes,
  listeners: {} as DraggableSyntheticListeners,
  setNodeRef: vi.fn(),
  transform: null,
  isDragging: false,
};

function renderCard(task: Task) {
  return render(
    <StateProvider>
      <KanbanCardShell
        {...dragProps}
        task={task}
        isPreviewed={false}
        menuEntries={[]}
        onClick={vi.fn()}
        onCheckboxClick={vi.fn()}
      />
    </StateProvider>,
  );
}

describe("KanbanCardShell running indicator", () => {
  it("shows the spinner when a review task still has a running primary session", () => {
    const { container } = renderCard({
      id: "task-running-review",
      title: "Review task with active agent",
      workflowStepId: "step-in-progress",
      state: "REVIEW",
      primarySessionId: "session-running",
      primarySessionState: "RUNNING",
    });

    expect(container.querySelector("svg.animate-spin")).not.toBeNull();
  });

  it("does not show the spinner when the primary session is completed", () => {
    const { container } = renderCard({
      id: "task-completed-review",
      title: "Review task with completed agent",
      workflowStepId: "step-in-progress",
      state: "REVIEW",
      primarySessionId: "session-completed",
      primarySessionState: "COMPLETED",
    });

    expect(container.querySelector("svg.animate-spin")).toBeNull();
  });
});
