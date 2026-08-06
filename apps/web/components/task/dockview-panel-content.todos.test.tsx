import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

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

// A session that has already completed by the time the panel mounts (page
// load, reopened task) only has its todo data in persisted message history —
// `sessionTodos.bySessionId` is populated exclusively by a live WS handler
// (`ws/handlers/session-todos.ts`) and is never backfilled from history. The
// panel must fall back to the same message-derived source `TodoIndicator`
// already uses (`buildTodoItems`), or it silently shows an empty state for
// every already-completed session — the exact regression a live manual
// smoke test caught (the chat status bar's todo chip showed "2/5" while this
// panel showed "No todos yet." for the identical session).
const mockMessages = vi.hoisted(() => ({ items: [] as Array<Record<string, unknown>> }));

vi.mock("@/hooks/domains/session/use-session-messages", () => ({
  useSessionMessages: () => ({ messages: mockMessages.items }),
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

afterEach(cleanup);

const WRITE_TESTS = "Write tests";
const IMPLEMENT_FEATURE = "Implement feature";

describe("dockview-panel-content todos panel (desktop task workbench)", () => {
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
    expect(screen.getByText("1/2 completed")).toBeTruthy();
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
