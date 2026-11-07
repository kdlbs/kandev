import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const available = vi.hoisted(() => ({ value: true }));
const openTaskGraph = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/domains/task/use-task-graph-available", () => ({
  useTaskGraphAvailable: () => available.value,
}));
vi.mock("@/hooks/domains/task/use-open-task-graph", () => ({
  useOpenTaskGraph: () => ({ openTab: openTaskGraph, showInGraph: vi.fn() }),
}));

import { TaskGraphButton } from "./task-graph-button";

afterEach(() => {
  cleanup();
  openTaskGraph.mockReset();
});

describe("TaskGraphButton", () => {
  it("renders nothing without relations", () => {
    available.value = false;
    render(<TaskGraphButton taskId="t1" />);
    expect(screen.queryByTestId("task-graph-button")).toBeNull();
  });

  it("opens the graph for its task", () => {
    available.value = true;
    render(<TaskGraphButton taskId="t1" />);
    fireEvent.click(screen.getByRole("button", { name: "Task graph" }));
    expect(openTaskGraph).toHaveBeenCalledWith("t1");
  });
});
