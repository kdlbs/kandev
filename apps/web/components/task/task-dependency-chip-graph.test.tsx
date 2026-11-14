import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

const touch = vi.hoisted(() => ({ value: false }));
const openTaskGraph = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => touch.value }));
vi.mock("@/hooks/domains/task/use-open-task-graph", () => ({
  useOpenTaskGraph: () => ({ openTab: openTaskGraph, showInGraph: vi.fn() }),
}));

import { TaskDependencyChip } from "./task-dependency-chip";

afterEach(() => {
  cleanup();
  openTaskGraph.mockReset();
});

const tasks = [
  {
    id: "b",
    workflowStepId: "s",
    title: "B",
    position: 0,
    dependsOn: [{ id: "a", title: "A", status: "pending" }],
    blocks: [],
  },
] as unknown as KanbanState["tasks"];

describe("TaskDependencyChip graph action", () => {
  it.each([false, true])("shows the task in the graph (touch=%s)", async (isTouch) => {
    touch.value = isTouch;
    render(
      <StateProvider initialState={{ kanban: { workflowId: "wf", steps: [], tasks } } as never}>
        <TaskDependencyChip taskId="b" />
      </StateProvider>,
    );
    fireEvent.click(screen.getByTestId("task-dependency-chip"));
    fireEvent.click(await screen.findByRole("button", { name: "View graph" }));
    expect(openTaskGraph).toHaveBeenCalledWith("b");
  });
});
