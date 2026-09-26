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

const { mockScrollTranscriptToMessage } = vi.hoisted(() => ({
  mockScrollTranscriptToMessage: vi.fn(),
}));

const promptHistoryProps = vi.hoisted(() => ({
  current: null as null | { onNavigateToPrompt?: (messageId: string) => void },
}));

vi.mock("@/hooks/domains/session/use-session-messages", () => ({
  useSessionMessages: () => ({ messages: mockMessages.items }),
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

vi.mock("./prompt-history-panel-content", () => ({
  PromptHistoryPanelContent: (props: { onNavigateToPrompt?: (messageId: string) => void }) => {
    promptHistoryProps.current = props;
    return <div data-testid="prompt-history-mock" />;
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(mockAppState),
  useAppStoreApi: () => ({ getState: () => mockAppState }),
}));

import { renderPanel, VALID_COMPONENTS } from "./dockview-shared";

afterEach(() => {
  cleanup();
  promptHistoryProps.current = null;
  mockScrollTranscriptToMessage.mockClear();
});

const WRITE_TESTS = "Write tests";
const IMPLEMENT_FEATURE = "Implement feature";
const SESSION_ID = "session-1";
const MESSAGE_ID = "message-1";
const CUSTOM_AGENT = "Custom Agent";
const PROMPT_HISTORY_COMPONENT = "prompt-history";
const PROMPT_HISTORY_MOCK = "prompt-history-mock";

describe("dockview prompt history panel (Office task layout)", () => {
  it("accepts the prompt-history component in the shared registry", () => {
    expect(VALID_COMPONENTS.has(PROMPT_HISTORY_COMPONENT)).toBe(true);
  });

  it("renders the prompt-history panel content for the registered component", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: { name: CUSTOM_AGENT } };

    render(<>{renderPanel(PROMPT_HISTORY_MOCK, PROMPT_HISTORY_COMPONENT, {})}</>);

    expect(screen.getByTestId(PROMPT_HISTORY_MOCK)).toBeTruthy();
  });

  it("binds the arrow to scrollTranscriptToMessage with a custom session name", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: { name: CUSTOM_AGENT } };

    render(<>{renderPanel(PROMPT_HISTORY_MOCK, PROMPT_HISTORY_COMPONENT, {})}</>);
    promptHistoryProps.current?.onNavigateToPrompt?.(MESSAGE_ID);

    expect(mockScrollTranscriptToMessage).toHaveBeenCalledWith(
      SESSION_ID,
      MESSAGE_ID,
      CUSTOM_AGENT,
    );
  });

  it("falls back to the chat panel title for an EMPTY-string session name", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: { name: "" } };

    render(<>{renderPanel(PROMPT_HISTORY_MOCK, PROMPT_HISTORY_COMPONENT, {})}</>);
    promptHistoryProps.current?.onNavigateToPrompt?.(MESSAGE_ID);

    expect(mockScrollTranscriptToMessage).toHaveBeenCalledWith(SESSION_ID, MESSAGE_ID, "Agent");
  });

  it("falls back to the chat panel title when the session name is absent", () => {
    mockAppState.taskSessions.items = { [SESSION_ID]: {} };

    render(<>{renderPanel(PROMPT_HISTORY_MOCK, PROMPT_HISTORY_COMPONENT, {})}</>);
    promptHistoryProps.current?.onNavigateToPrompt?.(MESSAGE_ID);

    expect(mockScrollTranscriptToMessage).toHaveBeenCalledWith(SESSION_ID, MESSAGE_ID, "Agent");
  });
});

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
