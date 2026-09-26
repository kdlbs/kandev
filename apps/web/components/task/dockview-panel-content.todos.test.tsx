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

const { mockScrollTranscriptToMessage } = vi.hoisted(() => ({
  mockScrollTranscriptToMessage: vi.fn(),
}));

const promptHistoryProps = vi.hoisted(() => ({
  current: null as null | { onNavigateToPrompt?: (messageId: string) => void },
}));

vi.mock("./prompt-history-panel-content", () => ({
  PromptHistoryPanelContent: (props: { onNavigateToPrompt?: (messageId: string) => void }) => {
    promptHistoryProps.current = props;
    return <div data-testid="prompt-history-mock" />;
  },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      selectedDiff: null,
      setSelectedDiff: vi.fn(),
      api: { getPanel: vi.fn(), removePanel: vi.fn() },
      scrollTranscriptToMessage: mockScrollTranscriptToMessage,
    }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(mockAppState),
  useAppStoreApi: () => ({ getState: () => mockAppState }),
}));

import { renderPanel } from "./dockview-panel-content";
import { TodosContent } from "./todos-panel-content";

afterEach(() => {
  cleanup();
  promptHistoryProps.current = null;
  mockScrollTranscriptToMessage.mockClear();
});

const WRITE_TESTS = "Write tests";
const IMPLEMENT_FEATURE = "Implement feature";
const SESSION_ID = "session-1";
// The dockview panel id equals the root testid; sharing one constant keeps
// the string under the lint duplicate-literal threshold.
const TODOS_PANEL_ID = "todos-panel";
const PROMPT_HISTORY_COMPONENT = "prompt-history";
const PROMPT_HISTORY_PANEL_TEST_ID = "prompt-history-panel";

describe("dockview-panel-content prompt history renderer (desktop task workbench)", () => {
  it("renders the prompt-history panel content for the registered component", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: { name: "Custom Agent" } };

    render(<>{renderPanel(PROMPT_HISTORY_PANEL_TEST_ID, PROMPT_HISTORY_COMPONENT, {})}</>);

    expect(screen.getByTestId("prompt-history-mock")).toBeTruthy();
  });

  it("binds the arrow to scrollTranscriptToMessage with a custom session name", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: { name: "Custom Agent" } };

    render(<>{renderPanel(PROMPT_HISTORY_PANEL_TEST_ID, PROMPT_HISTORY_COMPONENT, {})}</>);
    promptHistoryProps.current?.onNavigateToPrompt?.("message-1");

    expect(mockScrollTranscriptToMessage).toHaveBeenCalledWith(
      SESSION_ID,
      "message-1",
      "Custom Agent",
    );
  });

  it("falls back to the chat panel title for an EMPTY-string session name", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: { name: "" } };

    render(<>{renderPanel(PROMPT_HISTORY_PANEL_TEST_ID, PROMPT_HISTORY_COMPONENT, {})}</>);
    promptHistoryProps.current?.onNavigateToPrompt?.("message-1");

    expect(mockScrollTranscriptToMessage).toHaveBeenCalledWith(SESSION_ID, "message-1", "Agent");
  });

  it("falls back to the chat panel title when the session name is absent", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: {} };

    render(<>{renderPanel(PROMPT_HISTORY_PANEL_TEST_ID, PROMPT_HISTORY_COMPONENT, {})}</>);
    promptHistoryProps.current?.onNavigateToPrompt?.("message-1");

    expect(mockScrollTranscriptToMessage).toHaveBeenCalledWith(SESSION_ID, "message-1", "Agent");
  });
});

describe("dockview-panel-content todos panel (desktop task workbench)", () => {
  it("renders the same checklist rows, order, and status semantics the todo indicator shows", () => {
    mockAppState.sessionTodos.bySessionId = {
      "session-1": [
        { description: WRITE_TESTS, status: "completed" },
        { description: IMPLEMENT_FEATURE, status: "in_progress" },
      ],
    };
    mockMessages.items = [];

    render(<>{renderPanel(TODOS_PANEL_ID, "todos", {})}</>);

    const rows = screen.getAllByText(new RegExp(`${WRITE_TESTS}|${IMPLEMENT_FEATURE}`));
    expect(rows.map((row) => row.textContent)).toEqual([WRITE_TESTS, IMPLEMENT_FEATURE]);
    expect(screen.getByText("1/2 completed")).toBeTruthy();
    expect(screen.getByText(WRITE_TESTS).className).toContain("line-through");
    expect(screen.getByText(IMPLEMENT_FEATURE).className).not.toContain("line-through");
  });

  it("shows an empty state when the active session has no todo entries anywhere", () => {
    mockAppState.sessionTodos.bySessionId = {};
    mockMessages.items = [];

    render(<>{renderPanel(TODOS_PANEL_ID, "todos", {})}</>);

    expect(screen.getByTestId("todos-panel-empty-state").textContent).toBe("No todos yet.");
    // The empty state must occupy the full hosting panel like the populated
    // state (review finding, round 3), not collapse to its intrinsic height.
    expect(screen.getByTestId("todos-panel-empty-state").classList.contains("h-full")).toBe(true);
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

    render(<>{renderPanel(TODOS_PANEL_ID, "todos", {})}</>);

    const rows = screen.getAllByText(new RegExp(`${WRITE_TESTS}|${IMPLEMENT_FEATURE}`));
    expect(rows.map((row) => row.textContent)).toEqual([WRITE_TESTS, IMPLEMENT_FEATURE]);
    expect(screen.getByText("1/2 completed")).toBeTruthy();
    expect(screen.queryByTestId("todos-panel-empty-state")).toBeNull();
  });

  it("fills the hosting panel height instead of capping the checklist at the popover height", () => {
    mockAppState.sessionTodos.bySessionId = {
      "session-1": [
        { description: WRITE_TESTS, status: "completed" },
        { description: IMPLEMENT_FEATURE, status: "in_progress" },
      ],
    };
    mockMessages.items = [];

    render(<TodosContent />);

    // The pinned panel root must stretch to the portal slot's full height
    // (`h-full`), and its scroll container must not carry the popover-only
    // `max-h-48` cap — that cap is what left the lower part of the hosting
    // panel empty (the reported bug).
    const panel = screen.getByTestId(TODOS_PANEL_ID);
    expect(panel.classList.contains("h-full")).toBe(true);
    const list = panel.querySelector(".overflow-y-auto");
    expect(list).not.toBeNull();
    expect(list!.classList.contains("max-h-48")).toBe(false);
  });
});
