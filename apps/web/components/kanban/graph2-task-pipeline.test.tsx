import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { Graph2TaskPipeline, type Graph2TaskPipelineProps } from "./graph2-task-pipeline";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

afterEach(cleanup);

const STEPS: WorkflowStep[] = [
  { id: "todo", title: "Todo", color: "#64748b" },
  { id: "doing", title: "Doing", color: "#3b82f6" },
  { id: "done", title: "Done", color: "#22c55e" },
];

function makeTask(): Task {
  return {
    id: "task-1",
    title: "A task",
    workflowStepId: "doing",
  } as Task;
}

function renderPipeline(overrides: Partial<Graph2TaskPipelineProps> = {}) {
  const onPreviewTask = vi.fn();
  const onOpenTask = vi.fn();
  const onToggleSelect = vi.fn();
  render(
    <StateProvider>
      <TooltipProvider delayDuration={0}>
        <Graph2TaskPipeline
          task={makeTask()}
          steps={STEPS}
          moveTargetSteps={STEPS}
          onMoveTask={() => undefined}
          onPreviewTask={onPreviewTask}
          onOpenTask={onOpenTask}
          onDeleteTask={() => undefined}
          onArchiveTask={() => undefined}
          onToggleSelect={onToggleSelect}
          {...overrides}
        />
      </TooltipProvider>
    </StateProvider>,
  );
  return { onPreviewTask, onOpenTask, onToggleSelect };
}

describe("Graph2TaskPipeline — click routing (the regression that hid defect 1)", () => {
  it("routes a title click to onPreviewTask, not onOpenTask", () => {
    const { onPreviewTask, onOpenTask } = renderPipeline();

    fireEvent.click(screen.getByRole("button", { name: "A task" }));

    expect(onPreviewTask).toHaveBeenCalledWith(makeTask());
    expect(onOpenTask).not.toHaveBeenCalled();
  });

  it("routes a current-step pill click to onOpenTask, not onPreviewTask", () => {
    const { onPreviewTask, onOpenTask } = renderPipeline();

    fireEvent.click(screen.getByRole("button", { name: "Doing" }));

    expect(onOpenTask).toHaveBeenCalledWith(makeTask());
    expect(onPreviewTask).not.toHaveBeenCalled();
  });

  it("short-circuits a title click to onToggleSelect in multi-select mode", () => {
    const { onPreviewTask, onOpenTask, onToggleSelect } = renderPipeline({
      isMultiSelectMode: true,
    });

    fireEvent.click(screen.getByRole("button", { name: "A task" }));

    expect(onToggleSelect).toHaveBeenCalledWith("task-1");
    expect(onPreviewTask).not.toHaveBeenCalled();
    expect(onOpenTask).not.toHaveBeenCalled();
  });

  it("short-circuits a title click to onToggleSelect when already selected", () => {
    const { onPreviewTask, onToggleSelect } = renderPipeline({ isSelected: true });

    fireEvent.click(screen.getByRole("button", { name: "A task" }));

    expect(onToggleSelect).toHaveBeenCalledWith("task-1");
    expect(onPreviewTask).not.toHaveBeenCalled();
  });
});

describe("Graph2TaskPipeline — actions cluster stays reachable off-screen (defect 2)", () => {
  it("wraps the actions cluster in a sticky, right-pinned, opaque, full-height, above-pill-controls container", () => {
    renderPipeline();

    const wrapper = screen.getByTestId("pipeline-task-actions-sticky-task-1");
    expect(wrapper.className).toContain("sticky");
    expect(wrapper.className).toContain("right-0");
    expect(wrapper.className).toContain("bg-background");
    // z-20: must paint above the pill's z-10 move chevrons (F1).
    expect(wrapper.className).toContain("z-20");
    // self-stretch: must span the row's full content height, not just its own
    // content height, so no pill edge peeks out above/below the patch (F2).
    expect(wrapper.className).toContain("self-stretch");
  });

  it("does not render the sticky actions wrapper in multi-select mode", () => {
    renderPipeline({ isMultiSelectMode: true });

    expect(screen.queryByTestId("pipeline-task-actions-sticky-task-1")).toBeNull();
  });

  it("keeps the action trigger touch-sized on coarse pointers", () => {
    renderPipeline();

    const trigger = screen.getByTestId("pipeline-task-actions-trigger-task-1");
    expect(trigger.className).toContain("[@media(pointer:coarse)]:h-11");
    expect(trigger.className).toContain("[@media(pointer:coarse)]:w-11");
  });
});
