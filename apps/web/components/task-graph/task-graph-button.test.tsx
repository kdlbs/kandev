import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

const GRAPH_BUTTON_TEST_ID = "task-graph-button";

const available = vi.hoisted(() => ({ value: true }));
const openTaskGraph = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/domains/task/use-task-graph-available", () => ({
  useTaskGraphAvailable: () => available.value,
}));
vi.mock("@/hooks/domains/task/use-open-task-graph", () => ({
  useOpenTaskGraph: () => ({ openTab: openTaskGraph, showInGraph: vi.fn() }),
}));

import { TaskGraphButton } from "./task-graph-button";

function renderButton(taskId: string | null = "t1") {
  return render(
    <TooltipProvider>
      <TaskGraphButton taskId={taskId} />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  openTaskGraph.mockReset();
  available.value = true;
});

describe("TaskGraphButton", () => {
  it("renders nothing without relations", () => {
    available.value = false;
    renderButton();
    expect(screen.queryByTestId(GRAPH_BUTTON_TEST_ID)).toBeNull();
  });

  it("renders nothing without a task", () => {
    renderButton(null);
    expect(screen.queryByTestId(GRAPH_BUTTON_TEST_ID)).toBeNull();
  });

  it("renders a localized icon control without a visible text label", () => {
    renderButton();

    const button = screen.getByTestId(GRAPH_BUTTON_TEST_ID);
    expect(button.tagName).toBe("BUTTON");
    expect(button.getAttribute("type")).toBe("button");
    expect(button.getAttribute("aria-label")).toBe("Task graph");
    expect(button.textContent).toBe("");
    expect(screen.queryByText("Task graph")).toBeNull();
    expect(button.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
  });

  it("shows the accessible name in a tooltip on keyboard focus", async () => {
    renderButton();

    fireEvent.focus(screen.getByTestId(GRAPH_BUTTON_TEST_ID));

    expect((await screen.findByRole("tooltip")).textContent).toBe("Task graph");
  });

  it("opens the graph for its task", () => {
    renderButton();

    // Native buttons activate on click, Enter, and Space through the same action.
    fireEvent.click(screen.getByTestId(GRAPH_BUTTON_TEST_ID));

    expect(openTaskGraph).toHaveBeenCalledWith("t1");
  });
});
