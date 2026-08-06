import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

// Regression coverage for the real desktop task workbench's panel registry
// (`renderPanel` in this file), not the separate Office task layout's copy
// in `dockview-shared.tsx`. The two files intentionally maintain parallel
// panel registries (see `dockview-desktop-layout.tsx`'s `components` map vs
// `dockview-shared.tsx`'s `dockviewComponents`); a "todos" entry landing in
// one without the other silently breaks the desktop workbench while every
// unit test targeting the Office-layout copy stays green.
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

vi.mock("./task-changes-panel", () => ({
  TaskChangesPanel: () => <div data-testid="task-changes-panel" />,
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile: vi.fn() }),
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

import { renderPanel } from "./dockview-panel-content";

describe("dockview-panel-content todos panel (desktop task workbench)", () => {
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
    expect(screen.getByText("1/2 completed")).toBeTruthy();
    expect(screen.getByText("Write tests").className).toContain("line-through");
    expect(screen.getByText("Implement feature").className).not.toContain("line-through");
  });

  it("shows an empty state when the active session has no todo entries", () => {
    mockAppState.sessionTodos.bySessionId = {};

    render(<>{renderPanel("todos-panel", "todos", {})}</>);

    expect(screen.getByTestId("todos-panel-empty-state").textContent).toBe("No todos yet.");
  });
});
