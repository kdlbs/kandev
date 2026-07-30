import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { KanbanCardBody } from "./kanban-card-content";
import type { Task } from "./kanban-card";

vi.mock("@/components/github/pr-task-icon", () => ({
  PRTaskIcon: () => null,
}));

vi.mock("@/components/task/remote-cloud-tooltip", () => ({
  RemoteCloudTooltip: () => null,
}));

const task: Task = {
  id: "task-1",
  title: "A very long task title that should be fully visible inside the tooltip",
  workflowStepId: "step-1",
};

describe("KanbanCardBody", () => {
  it("shows the full task title in a tooltip", async () => {
    render(
      <StateProvider initialState={{ kanban: { workflowId: "wf-1", steps: [], tasks: [] } }}>
        <TooltipProvider>
          <KanbanCardBody task={task} repositoryChips={[]} />
        </TooltipProvider>
      </StateProvider>,
    );

    fireEvent.focus(screen.getByTestId("task-card-title"));

    expect((await screen.findByRole("tooltip")).textContent).toContain(task.title);
  });
});
