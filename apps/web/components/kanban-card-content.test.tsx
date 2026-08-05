import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mockAppState = {
  workspaces: { activeId: "ws-1" },
  kanban: { tasks: [] as Array<{ id: string; title: string }> },
  taskPRs: { byTaskId: {} as Record<string, unknown> },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
}));

vi.mock("@/components/github/pr-task-icon", () => ({
  PRTaskIcon: () => <span data-testid="pr-task-icon" />,
}));

import { pluginRegistry } from "@/lib/plugins/registry";
import { KanbanCardBody } from "./kanban-card-content";
import type { Task } from "./kanban-card";

const TASK: Task = {
  id: "task-1",
  title: "Fix the bug",
  workflowStepId: "step-1",
};

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("kandev-plugin-notes");
});

describe("KanbanCardBody — task-card-indicators slot", () => {
  it("renders no extra markup when no plugin is registered for the slot (AC14)", () => {
    const { container } = render(<KanbanCardBody task={TASK} repositoryChips={[]} />);
    expect(container.querySelector('[data-testid="plugin-indicator"]')).toBeNull();
  });

  it("renders a registered indicator with taskId/workspaceId/workflowStepId as slotProps (AC13)", () => {
    function Indicator({ slotProps }: { slotProps?: unknown }) {
      const props = slotProps as { taskId: string; workspaceId: string; workflowStepId: string };
      return (
        <span data-testid="plugin-indicator">
          {props.taskId}|{props.workspaceId}|{props.workflowStepId}
        </span>
      );
    }
    pluginRegistry
      .forPlugin("kandev-plugin-notes")
      .registerComponent("task-card-indicators", Indicator);

    render(<KanbanCardBody task={TASK} repositoryChips={[]} />);

    expect(screen.getByTestId("plugin-indicator").textContent).toBe("task-1|ws-1|step-1");
  });
});
