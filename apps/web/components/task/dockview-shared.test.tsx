import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

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

// See dockview-panel-content.todos.test.tsx for why this fallback exists: a
// session that already completed before the panel mounts only has its todo
// data in persisted message history, since `sessionTodos.bySessionId` is
// populated exclusively by a live WS handler and is never backfilled.
const mockMessages = vi.hoisted(() => ({ items: [] as Array<Record<string, unknown>> }));

vi.mock("@/hooks/domains/session/use-session-messages", () => ({
  useSessionMessages: () => ({ messages: mockMessages.items }),
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

afterEach(cleanup);

const WRITE_TESTS = "Write tests";
const IMPLEMENT_FEATURE = "Implement feature";

describe("dockview todos panel content", () => {
  it("renders the same checklist rows, order, and status semantics the todo indicator shows", () => {
    mockAppState.sessionTodos.bySessionId = {
      "session-1": [
        { description: WRITE_TESTS, status: "completed" },
        { description: IMPLEMENT_FEATURE, status: "in_progress" },
      ],
    };
    mockMessages.items = [];

    render(<>{renderPanel("todos-panel", "todos", {})}</>);

    const rows = screen.getAllByText(new RegExp(`${WRITE_TESTS}|${IMPLEMENT_FEATURE}`));
    expect(rows.map((row) => row.textContent)).toEqual([WRITE_TESTS, IMPLEMENT_FEATURE]);

    // Same progress header TodoIndicator shows: 1 of 2 entries completed.
    expect(screen.getByText("1/2 completed")).toBeTruthy();
    // Same status semantics: a completed row is struck through, the
    // in-progress row is not.
    expect(screen.getByText(WRITE_TESTS).className).toContain("line-through");
    expect(screen.getByText(IMPLEMENT_FEATURE).className).not.toContain("line-through");
  });

  it("shows an empty state when the active session has no todo entries anywhere", () => {
    mockAppState.sessionTodos.bySessionId = {};
    mockMessages.items = [];

    render(<>{renderPanel("todos-panel", "todos", {})}</>);

    expect(screen.getByTestId("todos-panel-empty-state").textContent).toBe("No todos yet.");
  });

  it("falls back to the latest persisted todo message when the live store has no entry for the session (a completed/reopened session)", () => {
    mockAppState.sessionTodos.bySessionId = {};
    mockMessages.items = [
      {
        id: "m1",
        type: "todo",
        turn_id: "turn-1",
        metadata: {
          todos: [
            { text: WRITE_TESTS, done: true, status: "completed" },
            { text: IMPLEMENT_FEATURE, done: false, status: "in_progress" },
          ],
        },
      },
    ];

    render(<>{renderPanel("todos-panel", "todos", {})}</>);

    const rows = screen.getAllByText(new RegExp(`${WRITE_TESTS}|${IMPLEMENT_FEATURE}`));
    expect(rows.map((row) => row.textContent)).toEqual([WRITE_TESTS, IMPLEMENT_FEATURE]);
    expect(screen.getByText("1/2 completed")).toBeTruthy();
    expect(screen.queryByTestId("todos-panel-empty-state")).toBeNull();
  });
});
