/* eslint-disable max-lines -- preview lifecycle and Plan-tab contracts share one store harness. */

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
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskPlan } from "@/lib/types/http-agents";

const mocks = vi.hoisted(() => ({
  taskChatPanelProps: null as null | Record<string, unknown>,
  sessions: [] as TaskSession[],
  agentProfiles: [] as AgentProfileOption[],
  primarySessionId: null as string | null,
  kanbanTasks: [{ id: "task-1", primarySessionId: null as string | null }],
  kanbanSnapshots: {} as Record<
    string,
    { tasks: Array<{ id: string; primarySessionId: string | null }> }
  >,
  setPrimary: vi.fn(),
  stop: vi.fn(),
  resume: vi.fn(),
  remove: vi.fn(async (_sessionId?: string, _options?: unknown) => true),
  renameSession: vi.fn(async () => undefined),
  upsertTaskSessionFromEvent: vi.fn(),
  taskSessionItems: {} as Record<string, TaskSession>,
  useTaskSessions: vi.fn(),
  useSessionResumption: vi.fn(),
  markTaskPlanSeen: vi.fn(),
  setTaskPlan: vi.fn(),
  setTaskPlanLoading: vi.fn(),
  getTaskPlan: vi.fn(),
}));

vi.mock("./task-chat-panel", () => ({
  TaskChatPanel: (props: Record<string, unknown>) => {
    mocks.taskChatPanelProps = props;
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
vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: ({
    sessionId,
    onDeleted,
  }: {
    sessionId?: string;
    onDeleted?: () => void;
  }) => ({
    setPrimary: () => mocks.setPrimary(sessionId),
    stop: () => mocks.stop(sessionId),
    resume: () => mocks.resume(sessionId),
    remove: async (options?: unknown) => {
      const ok = await mocks.remove(sessionId, options);
      if (ok) onDeleted?.();
      return ok;
    },
  }),
  isSessionStoppable: (s: string) =>
    s === "RUNNING" || s === "STARTING" || s === "WAITING_FOR_INPUT",
  isSessionDeletable: (s: string) => s !== "RUNNING" && s !== "STARTING",
  isSessionResumable: (s: string) => s === "COMPLETED" || s === "FAILED" || s === "CANCELLED",
}));
vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => (
    <span data-testid={`agent-logo-${agentName}`} />
  ),
}));
vi.mock("@/components/task/handoff-profile-menu-items", () => ({
  HandoffContextMenuSub: () => (
    <div role="menuitem" data-testid="session-handoff-submenu">
      Handoff
    </div>
  ),
}));
vi.mock("@/components/task/share/share-dialog", () => ({
  ShareDialog: () => null,
}));
vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));
vi.mock("@/lib/api/domains/session-api", () => ({
  renameSession: mocks.renameSession,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn(), updateToast: vi.fn() }),
}));
// Real TabRenameInput auto-focuses on mount inside a useEffect, which races
// against Radix's context-menu-close focus restoration under jsdom's
// synchronous focus/blur emulation (not reproducible in a real browser —
// the identical pattern already ships for terminal tabs). Stubbing it keeps
// this suite testing the preview's own wiring (open → commit through the
// shared rename hook), not TabRenameInput's unrelated focus management.
vi.mock("./tab-rename-input", () => ({
  TabRenameInput: ({
    initial,
    onCommit,
    testId,
  }: {
    initial: string;
    onCommit: (next: string) => void;
    testId?: string;
  }) => {
    let value = initial;
    return (
      <input
        data-testid={testId}
        defaultValue={initial}
        onChange={(e) => {
          value = e.target.value;
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") onCommit(value);
        }}
      />
    );
  },
}));

type TaskPlansState = {
  byTaskId: Record<string, TaskPlan | null>;
  loadedByTaskId: Record<string, boolean>;
  loadingByTaskId: Record<string, boolean>;
  lastSeenUpdatedAtByTaskId: Record<string, string | undefined>;
};

type FakeAppState = {
  agentProfiles: { items: AgentProfileOption[] };
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

type TestAppState = FakeAppState & {
  kanban: {
    tasks: Array<{ id: string; primarySessionId: string | null }>;
  };
  kanbanMulti: {
    snapshots: Record<string, { tasks: Array<{ id: string; primarySessionId: string | null }> }>;
  };
};

const appStoreApi = {
  getState: () => ({
    ...fakeStore.getState(),
    taskSessions: { items: mocks.taskSessionItems },
    upsertTaskSessionFromEvent: mocks.upsertTaskSessionFromEvent,
  }),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: TestAppState) => unknown) =>
    useStore(fakeStore, (state) =>
      selector({
        ...state,
        agentProfiles: { items: mocks.agentProfiles },
        kanban: {
          tasks: mocks.kanbanTasks.map((task) => ({
            ...task,
            primarySessionId: mocks.primarySessionId ?? task.primarySessionId,
          })),
        },
        kanbanMulti: { snapshots: mocks.kanbanSnapshots },
      }),
    ),
  useAppStoreApi: () => appStoreApi,
}));

import { PreviewSessionBody, PreviewSessionTabs } from "./preview-session-tabs";
import { usePreviewPlanSummary } from "./preview-plan-panel";

const TASK_ID = "task-1";
const TASK_ID_B = "task-2";
const TIMESTAMP = "2026-07-21T00:00:00Z";
const START_TIME = TIMESTAMP;
const LATER_TIMESTAMP = "2026-07-22T00:00:00Z";
const SESSION_A_TAB_TESTID = "preview-session-tab-session-a";
const PLAN_INDICATOR_TESTID = "preview-plan-tab-indicator";
const PLAN_TAB_TESTID = "preview-plan-tab";
const PLAN_PANEL_TESTID = "preview-plan-panel";
const PLAN_ERROR_TESTID = "preview-plan-error-state";

function makeSession(id: string, overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id,
    task_id: TASK_ID,
    state: "COMPLETED",
    started_at: START_TIME,
    updated_at: START_TIME,
    ...overrides,
  } as TaskSession;
}

const session: TaskSession = {
  id: toSessionId("session-1"),
  task_id: toTaskId(TASK_ID),
  state: "COMPLETED",
  started_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

afterEach(() => {
  cleanup();
  mocks.taskChatPanelProps = null;
  mocks.sessions = [];
  mocks.agentProfiles = [];
  mocks.primarySessionId = null;
  mocks.kanbanTasks = [{ id: "task-1", primarySessionId: null }];
  mocks.kanbanSnapshots = {};
  mocks.setPrimary.mockClear();
  mocks.stop.mockClear();
  mocks.resume.mockClear();
  mocks.remove.mockClear();
  mocks.remove.mockImplementation(async () => true);
  mocks.renameSession.mockClear();
  mocks.upsertTaskSessionFromEvent.mockClear();
  mocks.taskSessionItems = {};
  mocks.useTaskSessions.mockReset();
  mocks.useTaskSessions.mockImplementation(() => ({ sessions: mocks.sessions, isLoaded: true }));
  mocks.useSessionResumption.mockReset();
  mocks.useSessionResumption.mockImplementation(() => ({
    error: null,
    notice: null,
    resumeSession: vi.fn(),
  }));
  mocks.getTaskPlan.mockReset();
  mocks.getTaskPlan.mockResolvedValue(null);
  fakeStore.setState({ taskPlans: emptyTaskPlans(), connection: { status: "connected" } });
});

describe("PreviewSessionBody delivery", () => {
  it("uses TaskChatPanel's queue-aware shared delivery path", () => {
    render(<PreviewSessionBody session={session} taskId={TASK_ID} />);

    expect(mocks.taskChatPanelProps).toMatchObject({
      sessionId: "session-1",
      taskId: TASK_ID,
      hideSessionsDropdown: true,
    });
    expect(mocks.taskChatPanelProps).not.toHaveProperty("onSend");
  });
});

describe("PreviewSessionTabs tab label", () => {
  it("shows the session's custom name instead of the agent-derived label", () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED", name: "My renamed agent" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    expect(screen.getByTestId(SESSION_A_TAB_TESTID).textContent).toContain("My renamed agent");
  });
});

describe("PreviewSessionTabs session context menu", () => {
  it("renders the full lifecycle menu on right-click, without Close Others", () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));

    expect(screen.getByRole("menuitem", { name: "Rename…" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Set as Primary" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Resume" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Share" })).toBeTruthy();
    expect(screen.getByTestId("session-handoff-submenu")).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Close Others" })).toBeNull();
  });

  it("disables Set as Primary when the session is already primary", () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    mocks.primarySessionId = "session-a";
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));

    expect(
      screen.getByRole("menuitem", { name: "Set as Primary" }).getAttribute("aria-disabled"),
    ).toBe("true");
  });

  it("uses the latest primary session from a multi-workflow snapshot", () => {
    mocks.sessions = [
      makeSession("session-a", { state: "RUNNING" }),
      makeSession("session-b", { state: "RUNNING" }),
    ];
    mocks.kanbanTasks = [];
    mocks.kanbanSnapshots = {
      "workflow-2": { tasks: [{ id: TASK_ID, primarySessionId: "session-a" }] },
    };
    const { rerender } = render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-b" />);

    mocks.kanbanSnapshots = {
      "workflow-2": { tasks: [{ id: TASK_ID, primarySessionId: "session-b" }] },
    };
    rerender(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-b" />);
    fireEvent.contextMenu(screen.getByTestId("preview-session-tab-session-b"));

    expect(
      screen.getByRole("menuitem", { name: "Set as Primary" }).getAttribute("aria-disabled"),
    ).toBe("true");
  });

  it("shows Stop (not Resume) and hides Delete for a running session", () => {
    mocks.sessions = [makeSession("session-a", { state: "RUNNING" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));

    expect(screen.getByRole("menuitem", { name: "Stop" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Resume" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Delete" })).toBeNull();
  });

  it("opens the inline rename input and commits through the shared rename hook", async () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    mocks.taskSessionItems = { "session-a": mocks.sessions[0] };
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));
    fireEvent.click(screen.getByRole("menuitem", { name: "Rename…" }));

    const input = await screen.findByTestId("preview-session-tab-rename-input");
    fireEvent.change(input, { target: { value: "renamed session" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await vi.waitFor(() =>
      expect(mocks.renameSession).toHaveBeenCalledWith("session-a", "renamed session"),
    );
    expect(screen.queryByTestId("preview-session-tab-rename-input")).toBeNull();
  });

  it("opens the delete confirmation popover and calls session.delete on confirm", async () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    render(<PreviewSessionTabs taskId={TASK_ID} sessionId="session-a" />);

    fireEvent.contextMenu(screen.getByTestId(SESSION_A_TAB_TESTID));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    const confirm = await screen.findByTestId("session-delete-confirm");
    fireEvent.click(confirm);

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
  });
});

async function deleteViaContextMenu(tabTestId: string) {
  fireEvent.contextMenu(screen.getByTestId(tabTestId));
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
  const confirm = await screen.findByTestId("session-delete-confirm");
  fireEvent.click(confirm);
}

/** Makes `mocks.remove` hang until the returned resolver is called, so a
 * test can change props (simulate the user switching tabs) while a
 * `session.delete` round-trip is still in flight. */
function deferDelete() {
  let resolveDelete: (ok: boolean) => void = () => {};
  mocks.remove.mockImplementation(
    () =>
      new Promise<boolean>((resolve) => {
        resolveDelete = resolve;
      }),
  );
  return {
    settle: async (ok: boolean) => {
      await act(async () => {
        resolveDelete(ok);
        await Promise.resolve();
        await Promise.resolve();
      });
    },
  };
}

describe("PreviewSessionTabs delete session selection", () => {
  it("does not change the viewed session when a non-active tab is deleted", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
      makeSession("session-c", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-b"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu("preview-session-tab-session-a");

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
    expect(onSessionChange).not.toHaveBeenCalled();
  });

  it("re-points to a remaining session when the active tab is deleted", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu("preview-session-tab-session-a");

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
    expect(onSessionChange).toHaveBeenCalledWith("session-b");
  });

  it("calls onSessionChange with null when the last remaining session is deleted", async () => {
    mocks.sessions = [makeSession("session-a", { state: "COMPLETED" })];
    const onSessionChange = vi.fn();
    render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu(SESSION_A_TAB_TESTID);

    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );
    expect(onSessionChange).toHaveBeenCalledWith(null);
  });
});

describe("PreviewSessionTabs deferred delete", () => {
  it("does not redirect when the active tab changes while its own delete is still pending", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
      makeSession("session-c", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    const pendingDelete = deferDelete();
    const { rerender } = render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu(SESSION_A_TAB_TESTID);
    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-a", { feedback: "toast" }),
    );

    // User switches away from the tab being deleted before session.delete settles.
    rerender(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-b"
        onSessionChange={onSessionChange}
      />,
    );
    await pendingDelete.settle(true);

    expect(onSessionChange).not.toHaveBeenCalled();
  });

  it("redirects once the pending delete resolves for a tab that became active in the meantime", async () => {
    mocks.sessions = [
      makeSession("session-a", { state: "COMPLETED" }),
      makeSession("session-b", { state: "COMPLETED" }),
    ];
    const onSessionChange = vi.fn();
    const pendingDelete = deferDelete();
    const { rerender } = render(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-a"
        onSessionChange={onSessionChange}
      />,
    );

    await deleteViaContextMenu("preview-session-tab-session-b");
    await vi.waitFor(() =>
      expect(mocks.remove).toHaveBeenCalledWith("session-b", { feedback: "toast" }),
    );

    // User switches to the tab now pending deletion before session.delete settles.
    rerender(
      <PreviewSessionTabs
        taskId={TASK_ID}
        sessionId="session-b"
        onSessionChange={onSessionChange}
      />,
    );
    await pendingDelete.settle(true);

    expect(onSessionChange).toHaveBeenCalledWith("session-a");
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
