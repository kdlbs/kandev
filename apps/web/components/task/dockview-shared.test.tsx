import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const mockAppState = vi.hoisted(() => ({
  tasks: { activeSessionId: "session-1", activeTaskId: "task-1" },
  sessionTodos: {
    bySessionId: {} as Record<string, Array<{ description: string; status: string }>>,
  },
  taskSessions: { items: {} },
  agentProfiles: { items: [] },
  kanban: { tasks: [] },
  kanbanMulti: { snapshots: {} },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      selectedDiff: null,
      setSelectedDiff: vi.fn(),
      api: { getPanel: vi.fn(), removePanel: vi.fn() },
    }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(mockAppState),
  useAppStoreApi: () => ({ getState: () => mockAppState }),
}));

import { renderPanel } from "./dockview-shared";

describe("dockview todos panel content", () => {
  it("renders the same checklist rows, order, and status semantics the todo indicator shows", () => {
    mockAppState.sessionTodos.bySessionId = {
      "session-1": [
        { description: "Write tests", status: "completed" },
        { description: "Implement feature", status: "in_progress" },
      ],
    };

    render(<>{renderPanel("todos-panel", "todos", {})}</>);

    const rows = screen.getAllByText(/Write tests|Implement feature/);
    expect(rows.map((row) => row.textContent)).toEqual(["Write tests", "Implement feature"]);

    // Same progress header TodoIndicator shows: 1 of 2 entries completed.
    expect(screen.getByText("1/2 completed")).toBeTruthy();
    // Same status semantics: a completed row is struck through, the
    // in-progress row is not.
    expect(screen.getByText("Write tests").className).toContain("line-through");
    expect(screen.getByText("Implement feature").className).not.toContain("line-through");
  });

  it("shows an empty state when the active session has no todo entries", () => {
    mockAppState.sessionTodos.bySessionId = {};

    render(<>{renderPanel("todos-panel", "todos", {})}</>);

    expect(screen.getByTestId("todos-panel-empty-state").textContent).toBe("No todos yet.");
  });
});
