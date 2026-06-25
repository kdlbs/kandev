import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { TaskMultiSelectToolbar } from "./task-multi-select-toolbar";

function snapshots(): Record<string, WorkflowSnapshotData> {
  return {
    "workflow-1": {
      workflowId: "workflow-1",
      workflowName: "Build",
      steps: [],
      tasks: [
        {
          id: "task-1",
          workflowStepId: "step-1",
          title: "Task 1",
          position: 0,
          primaryExecutorType: "docker",
        },
      ],
    },
  };
}

describe("TaskMultiSelectToolbar", () => {
  it("renders selected-task actions from Query-owned snapshots without a Zustand store", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <TaskMultiSelectToolbar
          selectedIds={new Set(["task-1"])}
          snapshots={snapshots()}
          steps={[]}
          isProcessing={false}
          onClearSelection={vi.fn()}
          onBulkDelete={vi.fn()}
          onBulkArchive={vi.fn()}
          onBulkMove={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByTestId("multi-select-toolbar")).toBeTruthy();
    expect(screen.getByText("1 selected")).toBeTruthy();
  });
});
