import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { createStore } from "zustand/vanilla";
import { useStore } from "zustand";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { TaskPlan } from "@/lib/types/http-agents";
import type { ChatSubmitPayload } from "./chat/chat-input-container";
import type { EntityReference } from "@/lib/types/entity-reference";

const mocks = vi.hoisted(() => ({
  getWebSocketClient: vi.fn(),
  onSend: null as null | ((payload: ChatSubmitPayload) => Promise<void>),
  useTaskSessions: vi.fn(),
  useSessionResumption: vi.fn(),
  markTaskPlanSeen: vi.fn(),
  setTaskPlan: vi.fn(),
  setTaskPlanLoading: vi.fn(),
  getTaskPlan: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));
vi.mock("./task-chat-panel", () => ({
  TaskChatPanel: ({ onSend }: { onSend: (payload: ChatSubmitPayload) => Promise<void> }) => {
    mocks.onSend = onSend;
    return <div data-testid="preview-chat" />;
  },
}));
vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: mocks.useTaskSessions,
}));
vi.mock("@/hooks/domains/session/use-session-resumption", () => ({
  useSessionResumption: mocks.useSessionResumption,
}));
vi.mock("@/lib/api/domains/plan-api", () => ({
  getTaskPlan: mocks.getTaskPlan,
}));
vi.mock("@/components/editors/tiptap/tiptap-plan-readonly", () => ({
  PlanReadOnlyMarkdown: (props: { content: string; testId?: string }) => (
    <div data-testid={props.testId}>{props.content}</div>
  ),
}));

type TaskPlansState = {
  byTaskId: Record<string, TaskPlan | null>;
  loadedByTaskId: Record<string, boolean>;
  loadingByTaskId: Record<string, boolean>;
  lastSeenUpdatedAtByTaskId: Record<string, string | undefined>;
};

type FakeAppState = {
  agentProfiles: { items: never[] };
  connection: { status: "connected" | "disconnected" };
  taskPlans: TaskPlansState;
  markTaskPlanSeen: (taskId: string) => void;
  setTaskPlan: (taskId: string, plan: TaskPlan | null) => void;
  setTaskPlanLoading: (taskId: string, loading: boolean) => void;
};

function emptyTaskPlans(): TaskPlansState {
  return { byTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {}, lastSeenUpdatedAtByTaskId: {} };
}

// A real (non-static) store double: production code drives loading/loaded
// state through these actions, and components re-render on real state
// changes, exactly like the app's actual Zustand store. This is required to
// exercise effect-retrigger regressions (F1/F2) that a frozen mock object
// cannot reproduce.
const fakeStore = createStore<FakeAppState>()((set, get) => ({
  agentProfiles: { items: [] },
  connection: { status: "connected" },
  taskPlans: emptyTaskPlans(),
  markTaskPlanSeen: (taskId: string) => {
    mocks.markTaskPlanSeen(taskId);
    const plan = get().taskPlans.byTaskId[taskId];
    set((state) => ({
      taskPlans: {
        ...state.taskPlans,
        lastSeenUpdatedAtByTaskId: {
          ...state.taskPlans.lastSeenUpdatedAtByTaskId,
          [taskId]: plan?.updated_at ?? "",
        },
      },
    }));
  },
  setTaskPlan: (taskId: string, plan: TaskPlan | null) => {
    mocks.setTaskPlan(taskId, plan);
    set((state) => ({
      taskPlans: {
        ...state.taskPlans,
        byTaskId: { ...state.taskPlans.byTaskId, [taskId]: plan },
        loadingByTaskId: { ...state.taskPlans.loadingByTaskId, [taskId]: false },
        loadedByTaskId: { ...state.taskPlans.loadedByTaskId, [taskId]: true },
      },
    }));
  },
  setTaskPlanLoading: (taskId: string, loading: boolean) => {
    mocks.setTaskPlanLoading(taskId, loading);
    set((state) => ({
      taskPlans: {
        ...state.taskPlans,
        loadingByTaskId: { ...state.taskPlans.loadingByTaskId, [taskId]: loading },
      },
    }));
  },
}));

function setTaskPlansState(patch: Partial<TaskPlansState>) {
  fakeStore.setState((state) => ({ taskPlans: { ...state.taskPlans, ...patch } }));
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: FakeAppState) => unknown) => useStore(fakeStore, selector),
  useAppStoreApi: () => fakeStore,
}));

import { PreviewSessionBody, PreviewSessionTabs } from "./preview-session-tabs";
import { usePreviewPlanSummary } from "./preview-plan-panel";

const TASK_ID = "task-1";
const TASK_ID_B = "task-2";
const TIMESTAMP = "2026-07-21T00:00:00Z";
const LATER_TIMESTAMP = "2026-07-22T00:00:00Z";
const PLAN_INDICATOR_TESTID = "preview-plan-tab-indicator";
const PLAN_TAB_TESTID = "preview-plan-tab";
const PLAN_PANEL_TESTID = "preview-plan-panel";
const PLAN_ERROR_TESTID = "preview-plan-error-state";

const session: TaskSession = {
  id: toSessionId("session-1"),
  task_id: toTaskId(TASK_ID),
  state: "COMPLETED",
  started_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

afterEach(() => {
  cleanup();
  mocks.getWebSocketClient.mockReset();
  mocks.onSend = null;
});

describe("PreviewSessionBody send failures", () => {
  it("rejects when the WebSocket client is unavailable", async () => {
    mocks.getWebSocketClient.mockReturnValue(null);
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

    await expect(mocks.onSend?.({ message: "hello" })).rejects.toMatchObject({
      name: "MessageSendError",
      code: "connection-unavailable",
      message: "Connection unavailable. Reconnect and try again.",
    });
  });

  it("rethrows message.add failures to the chat input", async () => {
    const error = new Error("message.add failed");
    mocks.getWebSocketClient.mockReturnValue({ request: vi.fn().mockRejectedValue(error) });
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

    await expect(mocks.onSend?.({ message: "hello" })).rejects.toBe(error);
  });

  it("forwards attachments and entity references through preview direct send", async () => {
    const request = vi.fn().mockResolvedValue(undefined);
    mocks.getWebSocketClient.mockReturnValue({ request });
    const reference: EntityReference = {
      version: 1,
      ref: "mention:v1:github:issue:acme%2Frepo:42",
      provider: "github",
      kind: "issue",
      id: "42",
      key: "acme/repo#42",
      title: "Fix composer references",
      url: "https://github.com/acme/repo/issues/42",
      scope: "acme/repo",
    };
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

    await mocks.onSend?.({
      message: "reference",
      attachments: [{ type: "image", data: "base64", mime_type: "image/png" }],
      entityReferences: [reference],
    });

    expect(request).toHaveBeenCalledWith(
      "message.add",
      {
        task_id: TASK_ID,
        session_id: "session-1",
        client_message_id: expect.any(String),
        content: "reference",
        attachments: [{ type: "image", data: "base64", mime_type: "image/png" }],
        entity_references: [reference],
      },
      30000,
    );
  });
});

function agentPlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content: "## Plan content",
    created_by: "agent",
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

describe("PreviewSessionTabs Plan tab", () => {
  beforeEach(() => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [session], isLoaded: true });
    mocks.useSessionResumption.mockReturnValue({
      error: null,
      notice: null,
      resumeSession: vi.fn(),
    });
    mocks.getTaskPlan.mockResolvedValue(null);
    fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
  });

  afterEach(() => {
    cleanup();
    mocks.useTaskSessions.mockReset();
    mocks.useSessionResumption.mockReset();
    mocks.markTaskPlanSeen.mockReset();
    mocks.getTaskPlan.mockReset();
  });

  it("shows a Plan tab alongside session tabs", () => {
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.getByTestId(PLAN_TAB_TESTID)).toBeTruthy();
    expect(screen.getByTestId(`preview-session-tab-${session.id}`)).toBeTruthy();
  });

  it("clicking the Plan tab swaps the chat body for the read-only plan and does not change the session", () => {
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan() },
      loadedByTaskId: { [TASK_ID]: true },
    });
    const onSessionChange = vi.fn();

    render(
      <PreviewSessionTabs taskId={TASK_ID} sessionId={null} onSessionChange={onSessionChange} />,
    );
    expect(screen.getByTestId("preview-chat")).toBeTruthy();

    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    expect(screen.queryByTestId("preview-chat")).toBeNull();
    expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    expect(screen.getByText("## Plan content")).toBeTruthy();
    expect(onSessionChange).not.toHaveBeenCalled();
  });

  it("shows the unseen indicator for an agent-authored plan the user hasn't seen", () => {
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan() },
      loadedByTaskId: { [TASK_ID]: true },
    });

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.getByTestId(PLAN_INDICATOR_TESTID)).toBeTruthy();
  });

  it("does not show the indicator for a user-authored plan", () => {
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan({ created_by: "user" }) },
      loadedByTaskId: { [TASK_ID]: true },
    });

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("does not show the indicator once the plan's updated_at has been seen", () => {
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan() },
      loadedByTaskId: { [TASK_ID]: true },
      lastSeenUpdatedAtByTaskId: { [TASK_ID]: TIMESTAMP },
    });

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("shows the Plan tab and renders the plan when the task has no sessions", () => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [], isLoaded: true });
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan() },
      loadedByTaskId: { [TASK_ID]: true },
    });

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.getByTestId(PLAN_TAB_TESTID)).toBeTruthy();
    expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    expect(screen.queryByTestId("preview-empty-state")).toBeNull();
    expect(mocks.markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("uses a touch-sized tab list for coarse-pointer previews", () => {
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    const list = screen
      .getByTestId("preview-session-tabs")
      .querySelector('[data-slot="tabs-list"]');
    expect(list).not.toBeNull();
    expect(list?.className).toContain("[@media(pointer:coarse)]:!h-11");
  });

  it("clears the indicator (marks seen) when the Plan tab is clicked", () => {
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan() },
      loadedByTaskId: { [TASK_ID]: true },
    });

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    expect(screen.getByTestId(PLAN_INDICATOR_TESTID)).toBeTruthy();

    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    expect(mocks.markTaskPlanSeen).toHaveBeenCalledWith(TASK_ID);
  });
});

describe("PreviewSessionTabs Plan tab session loading", () => {
  beforeEach(() => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [], isLoaded: false });
    mocks.useSessionResumption.mockReturnValue({
      error: null,
      notice: null,
      resumeSession: vi.fn(),
    });
    mocks.getTaskPlan.mockResolvedValue(null);
    fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
  });

  afterEach(() => {
    cleanup();
    mocks.useTaskSessions.mockReset();
    mocks.useSessionResumption.mockReset();
    mocks.markTaskPlanSeen.mockReset();
    mocks.getTaskPlan.mockReset();
  });

  it("does not mark a plan seen while the session list is still loading", () => {
    setTaskPlansState({
      byTaskId: { [TASK_ID]: agentPlan() },
      loadedByTaskId: { [TASK_ID]: true },
    });

    const { rerender } = render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(mocks.markTaskPlanSeen).not.toHaveBeenCalled();

    mocks.useTaskSessions.mockReturnValue({ sessions: [session], isLoaded: true });
    rerender(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);

    expect(screen.getByTestId(PLAN_INDICATOR_TESTID)).toBeTruthy();
  });
});

describe("PreviewSessionTabs Plan tab reliability", () => {
  beforeEach(() => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [session], isLoaded: true });
    mocks.useSessionResumption.mockReturnValue({
      error: null,
      notice: null,
      resumeSession: vi.fn(),
    });
    mocks.getTaskPlan.mockResolvedValue(null);
    fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
  });

  afterEach(() => {
    cleanup();
    mocks.useTaskSessions.mockReset();
    mocks.useSessionResumption.mockReset();
    mocks.markTaskPlanSeen.mockReset();
    mocks.getTaskPlan.mockReset();
  });

  it("stops retrying the plan fetch after it rejects and shows an error state", async () => {
    mocks.getTaskPlan.mockRejectedValue(new Error("boom"));

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_ERROR_TESTID)).toBeTruthy();
    });
    expect(mocks.getTaskPlan).toHaveBeenCalledTimes(1);

    // A retry-loop regression would call getTaskPlan again on every one of
    // these render/microtask cycles.
    await act(async () => {
      for (let i = 0; i < 5; i += 1) await Promise.resolve();
    });

    expect(mocks.getTaskPlan).toHaveBeenCalledTimes(1);
  });

  it("retries the plan fetch for a task revisited after a failure", async () => {
    // `PreviewSessionTabs` is reused across tasks without remounting, so a
    // failure for TASK_ID must survive switching the preview to TASK_ID_B and
    // back. Only TASK_ID's fetch rejects (once); TASK_ID_B always resolves.
    mocks.getTaskPlan.mockImplementation((id: string) =>
      id === TASK_ID ? Promise.reject(new Error("boom")) : Promise.resolve(null),
    );

    const { rerender } = render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_ERROR_TESTID)).toBeTruthy();
    });
    expect(mocks.getTaskPlan.mock.calls.filter(([id]) => id === TASK_ID)).toHaveLength(1);

    // Switch away to another task, then back to the failed one.
    act(() => {
      rerender(<PreviewSessionTabs taskId={TASK_ID_B} sessionId={null} />);
    });
    mocks.getTaskPlan.mockImplementation((id: string) =>
      id === TASK_ID ? Promise.resolve(agentPlan()) : Promise.resolve(null),
    );
    act(() => {
      rerender(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    });

    // Re-selecting the Plan tab is the retry signal.
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    });
    expect(mocks.getTaskPlan.mock.calls.filter(([id]) => id === TASK_ID)).toHaveLength(2);
  });
});

describe("PreviewSessionTabs Plan tab recovery", () => {
  beforeEach(() => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [session], isLoaded: true });
    mocks.useSessionResumption.mockReturnValue({
      error: null,
      notice: null,
      resumeSession: vi.fn(),
    });
    mocks.getTaskPlan.mockResolvedValue(null);
    fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
  });

  afterEach(() => {
    cleanup();
    mocks.useTaskSessions.mockReset();
    mocks.useSessionResumption.mockReset();
    mocks.markTaskPlanSeen.mockReset();
    mocks.getTaskPlan.mockReset();
  });

  it("retries a failed plan fetch after the WebSocket reconnects", async () => {
    mocks.getTaskPlan.mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce(agentPlan());

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_ERROR_TESTID)).toBeTruthy();
    });

    act(() => {
      fakeStore.setState({ connection: { status: "disconnected" } });
    });
    act(() => {
      fakeStore.setState({ connection: { status: "connected" } });
    });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    });
    expect(mocks.getTaskPlan).toHaveBeenCalledTimes(2);
  });

  it("shows an authoritative WebSocket plan after an earlier fetch failure", async () => {
    mocks.getTaskPlan.mockRejectedValue(new Error("boom"));

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_ERROR_TESTID)).toBeTruthy();
    });

    act(() => {
      fakeStore.getState().setTaskPlan(TASK_ID, agentPlan({ updated_at: LATER_TIMESTAMP }));
    });

    expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(PLAN_ERROR_TESTID)).toBeNull();
  });

  it("retries a failed plan fetch from the error state", async () => {
    mocks.getTaskPlan.mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce(agentPlan());

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId("preview-plan-retry")).toBeTruthy();
    });
    fireEvent.click(screen.getByTestId("preview-plan-retry"));

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    });
    expect(mocks.getTaskPlan).toHaveBeenCalledTimes(2);
  });
});

describe("PreviewSessionTabs Plan tab indicator timing", () => {
  beforeEach(() => {
    mocks.useTaskSessions.mockReturnValue({ sessions: [session], isLoaded: true });
    mocks.useSessionResumption.mockReturnValue({
      error: null,
      notice: null,
      resumeSession: vi.fn(),
    });
    mocks.getTaskPlan.mockResolvedValue(null);
    fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
  });

  afterEach(() => {
    cleanup();
    mocks.useTaskSessions.mockReset();
    mocks.useSessionResumption.mockReset();
    mocks.markTaskPlanSeen.mockReset();
    mocks.getTaskPlan.mockReset();
  });

  it("does not re-light the indicator when the plan finishes loading while the Plan tab is already open", async () => {
    mocks.getTaskPlan.mockResolvedValue(agentPlan());

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });

    await waitFor(() => {
      expect(screen.getByTestId(PLAN_PANEL_TESTID)).toBeTruthy();
    });

    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("does not re-light the indicator when the plan updates over WS while the Plan tab is open", () => {
    const initialPlan = agentPlan();
    setTaskPlansState({
      byTaskId: { [TASK_ID]: initialPlan },
      loadedByTaskId: { [TASK_ID]: true },
      lastSeenUpdatedAtByTaskId: { [TASK_ID]: initialPlan.updated_at },
    });

    render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });
    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();

    // Simulate a task.plan.updated WS event landing for this task while the
    // Plan tab is already the active view.
    const updatedPlan = agentPlan({ updated_at: LATER_TIMESTAMP });
    act(() => {
      setTaskPlansState({ byTaskId: { [TASK_ID]: updatedPlan } });
    });

    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeNull();
  });

  it("does not mark the next task's plan seen when switching the previewed task while Plan is open", () => {
    setTaskPlansState({
      byTaskId: {
        [TASK_ID]: agentPlan(),
        [TASK_ID_B]: agentPlan({
          id: "plan-2",
          task_id: TASK_ID_B,
          updated_at: LATER_TIMESTAMP,
        }),
      },
      loadedByTaskId: { [TASK_ID]: true, [TASK_ID_B]: true },
    });

    const { rerender } = render(<PreviewSessionTabs taskId={TASK_ID} sessionId={null} />);
    fireEvent.mouseDown(screen.getByTestId(PLAN_TAB_TESTID), { button: 0 });
    mocks.markTaskPlanSeen.mockReset();

    act(() => {
      rerender(<PreviewSessionTabs taskId={TASK_ID_B} sessionId={null} />);
    });

    expect(mocks.markTaskPlanSeen).not.toHaveBeenCalledWith(TASK_ID_B);
    expect(fakeStore.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK_ID_B]).toBeUndefined();
    expect(screen.queryByTestId(PLAN_INDICATOR_TESTID)).toBeTruthy();
  });
});

describe("usePreviewPlanSummary fetch race", () => {
  beforeEach(() => {
    fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
  });

  afterEach(() => {
    mocks.getTaskPlan.mockReset();
    mocks.setTaskPlan.mockReset();
  });

  it("does not overwrite a newer WS-pushed plan with a stale null fetch response", async () => {
    let resolveFetch: (value: TaskPlan | null) => void = () => {};
    mocks.getTaskPlan.mockReturnValue(
      new Promise<TaskPlan | null>((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => usePreviewPlanSummary(TASK_ID));

    const newerPlan = agentPlan({ updated_at: LATER_TIMESTAMP });
    act(() => {
      fakeStore.getState().setTaskPlan(TASK_ID, newerPlan);
    });

    await act(async () => {
      resolveFetch(null);
      await Promise.resolve();
    });

    expect(fakeStore.getState().taskPlans.byTaskId[TASK_ID]).toEqual(newerPlan);
    expect(fakeStore.getState().taskPlans.loadingByTaskId[TASK_ID]).toBe(false);
  });

  it("does not overwrite a newer WS-pushed plan with a stale, older fetch response", async () => {
    let resolveFetch: (value: TaskPlan | null) => void = () => {};
    mocks.getTaskPlan.mockReturnValue(
      new Promise<TaskPlan | null>((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => usePreviewPlanSummary(TASK_ID));

    const newerPlan = agentPlan({ updated_at: LATER_TIMESTAMP });
    act(() => {
      fakeStore.getState().setTaskPlan(TASK_ID, newerPlan);
    });

    const staleFetchedPlan = agentPlan({ updated_at: TIMESTAMP });
    await act(async () => {
      resolveFetch(staleFetchedPlan);
      await Promise.resolve();
    });

    expect(fakeStore.getState().taskPlans.byTaskId[TASK_ID]).toEqual(newerPlan);
    expect(fakeStore.getState().taskPlans.loadingByTaskId[TASK_ID]).toBe(false);
  });

  it("does not resurrect a plan deleted over WS while the fetch was in flight", async () => {
    let resolveFetch: (value: TaskPlan | null) => void = () => {};
    mocks.getTaskPlan.mockReturnValue(
      new Promise<TaskPlan | null>((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => usePreviewPlanSummary(TASK_ID));

    act(() => {
      fakeStore.getState().setTaskPlan(TASK_ID, null);
      fakeStore.getState().markTaskPlanSeen(TASK_ID);
    });

    const staleFetchedPlan = agentPlan({ updated_at: TIMESTAMP });
    await act(async () => {
      resolveFetch(staleFetchedPlan);
      await Promise.resolve();
    });

    expect(fakeStore.getState().taskPlans.byTaskId[TASK_ID]).toBeNull();
    expect(fakeStore.getState().taskPlans.loadingByTaskId[TASK_ID]).toBe(false);
  });
});
