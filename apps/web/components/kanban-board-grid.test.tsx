import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { KanbanBoardGrid } from "./kanban-board-grid";
import type { WorkflowStep } from "./kanban-column";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => false,
}));

const noop = vi.fn();

function renderGrid() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const steps: WorkflowStep[] = [
    {
      id: "step-1",
      title: "Query Ready",
      color: "bg-blue-500",
    },
  ];

  return render(
    <QueryClientProvider client={client}>
      <StateProvider
        initialState={{
          workflows: { activeId: "workflow-1" },
        }}
      >
        <KanbanBoardGrid
          steps={steps}
          tasks={[]}
          onPreviewTask={noop}
          onOpenTask={noop}
          onEditTask={noop}
          onDeleteTask={noop}
          onDragStart={noop}
          onDragEnd={noop}
          onDragCancel={noop}
          activeTask={null}
          isLoading={false}
        />
      </StateProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  noop.mockClear();
});

describe("KanbanBoardGrid", () => {
  it("does not show loading when Query-derived steps are present", () => {
    renderGrid();

    expect(screen.queryByText("Loading...")).toBeNull();
    expect(screen.getByTestId("kanban-column-step-1")).toBeTruthy();
  });
});
