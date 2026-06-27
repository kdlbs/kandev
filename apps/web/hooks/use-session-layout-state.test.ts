import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskPlan,
  type TaskSession,
} from "@/lib/types/http";

const mockSetTaskSession = vi.fn();
const mockSetMobileSessionPanel = vi.fn();
const mockSetMobileSessionTaskSwitcherOpen = vi.fn();
const planApiMocks = vi.hoisted(() => ({
  getPlanRevision: vi.fn(),
  getTaskPlan: vi.fn(),
  listPlanRevisions: vi.fn(),
}));

type MockStoreState = {
  tasks: {
    activeTaskId: string | null;
    activeSessionId: string | null;
    lastSessionByTaskId: Record<string, string>;
  };
  taskSessions: {
    items: Record<string, TaskSession>;
  };
  taskPlans: {
    byTaskId: Record<string, unknown>;
  };
  mobileSession: {
    activePanelBySessionId: Record<string, "chat" | "changes" | "terminal" | "plan">;
    isTaskSwitcherOpen: boolean;
  };
  setTaskSession: typeof mockSetTaskSession;
  setMobileSessionPanel: typeof mockSetMobileSessionPanel;
  setMobileSessionTaskSwitcherOpen: typeof mockSetMobileSessionTaskSwitcherOpen;
};

let mockStoreState: MockStoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockStoreState) => unknown) => selector(mockStoreState),
}));

vi.mock("@/hooks/domains/session/use-session-changes-count", () => ({
  useSessionChangesCount: () => 0,
}));

vi.mock("@/hooks/domains/session/use-session", () => ({
  useSession: (sessionId: string | null) => ({
    session: sessionId ? (mockStoreState.taskSessions.items[sessionId] ?? null) : null,
    isActive: false,
    isFailed: false,
    errorMessage: undefined,
  }),
}));

vi.mock("@/lib/local-storage", () => ({
  getPlanLastSeen: () => null,
}));

vi.mock("@/lib/api/domains/plan-api", () => ({
  getPlanRevision: planApiMocks.getPlanRevision,
  getTaskPlan: planApiMocks.getTaskPlan,
  listPlanRevisions: planApiMocks.listPlanRevisions,
}));

vi.mock("@/lib/services/session-approve", () => ({
  executeApprove: vi.fn(),
}));

vi.mock("@/lib/session/is-passthrough-session", () => ({
  isPassthroughSession: () => false,
}));

import { useSessionLayoutState } from "./use-session-layout-state";

function renderLayoutHook(options?: Parameters<typeof useSessionLayoutState>[0]) {
  const queryClient = makeQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return renderHook(() => useSessionLayoutState(options), { wrapper });
}

const ACTIVE_TASK_ID = "task-active";
const OTHER_TASK_ID = "task-other";
const ACTIVE_SESSION_ID = "session-active";
const ROUTE_SESSION_ID = "session-route";

function makeSession(id: string, taskId: string): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(taskId),
    state: "WAITING_FOR_INPUT",
    created_at: "",
    started_at: "",
    updated_at: "",
  } as TaskSession;
}

function makePlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: ACTIVE_TASK_ID,
    title: "Plan",
    content: "Do the work",
    created_by: "agent",
    created_at: "2026-06-24T00:00:00Z",
    updated_at: "2026-06-24T00:01:00Z",
    ...overrides,
  };
}

function setupStore(overrides: Partial<MockStoreState> = {}) {
  mockStoreState = {
    tasks: {
      activeTaskId: ACTIVE_TASK_ID,
      activeSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessions: {
      items: {},
    },
    taskPlans: {
      byTaskId: {},
    },
    mobileSession: {
      activePanelBySessionId: {},
      isTaskSwitcherOpen: false,
    },
    setTaskSession: mockSetTaskSession,
    setMobileSessionPanel: mockSetMobileSessionPanel,
    setMobileSessionTaskSwitcherOpen: mockSetMobileSessionTaskSwitcherOpen,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  planApiMocks.getTaskPlan.mockResolvedValue(null);
  setupStore();
});

describe("useSessionLayoutState active session selection", () => {
  it("uses the active session when it belongs to the active task", () => {
    setupStore({
      tasks: {
        activeTaskId: ACTIVE_TASK_ID,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [ACTIVE_SESSION_ID]: makeSession(ACTIVE_SESSION_ID, ACTIVE_TASK_ID),
        },
      },
    });

    const { result } = renderLayoutHook({ sessionId: ROUTE_SESSION_ID });

    expect(result.current.effectiveSessionId).toBe(ACTIVE_SESSION_ID);
  });

  it("falls back to the provided session when the active session belongs to another task", () => {
    setupStore({
      tasks: {
        activeTaskId: ACTIVE_TASK_ID,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [ACTIVE_SESSION_ID]: makeSession(ACTIVE_SESSION_ID, OTHER_TASK_ID),
        },
      },
    });

    const { result } = renderLayoutHook({ sessionId: ROUTE_SESSION_ID });

    expect(result.current.effectiveSessionId).toBe(ROUTE_SESSION_ID);
  });

  it("does not expose a stale active session when no session fallback exists", () => {
    setupStore({
      tasks: {
        activeTaskId: ACTIVE_TASK_ID,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [ACTIVE_SESSION_ID]: makeSession(ACTIVE_SESSION_ID, OTHER_TASK_ID),
        },
      },
    });

    const { result } = renderLayoutHook();

    expect(result.current.effectiveSessionId).toBeNull();
  });

  it("does not expose an active session when no task is active", () => {
    setupStore({
      tasks: {
        activeTaskId: null,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [ACTIVE_SESSION_ID]: makeSession(ACTIVE_SESSION_ID, ACTIVE_TASK_ID),
        },
      },
    });

    const { result } = renderLayoutHook();

    expect(result.current.effectiveSessionId).toBeNull();
  });
});

describe("useSessionLayoutState rowless active sessions", () => {
  it("uses a rowless active session when it was selected for the active task", () => {
    setupStore({
      tasks: {
        activeTaskId: ACTIVE_TASK_ID,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {
          [ACTIVE_TASK_ID]: ACTIVE_SESSION_ID,
        },
      },
      taskSessions: {
        items: {},
      },
    });

    const { result } = renderLayoutHook({ sessionId: ROUTE_SESSION_ID });

    expect(result.current.effectiveSessionId).toBe(ACTIVE_SESSION_ID);
  });

  it("does not expose a rowless active session without task ownership evidence", () => {
    setupStore({
      tasks: {
        activeTaskId: ACTIVE_TASK_ID,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {
          [OTHER_TASK_ID]: ACTIVE_SESSION_ID,
        },
      },
      taskSessions: {
        items: {},
      },
    });

    const { result } = renderLayoutHook({ sessionId: ROUTE_SESSION_ID });

    expect(result.current.effectiveSessionId).toBe(ROUTE_SESSION_ID);
  });

  it("falls back when no task is active", () => {
    setupStore({
      tasks: {
        activeTaskId: null,
        activeSessionId: ACTIVE_SESSION_ID,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [ACTIVE_SESSION_ID]: makeSession(ACTIVE_SESSION_ID, ACTIVE_TASK_ID),
        },
      },
    });

    const { result } = renderLayoutHook({ sessionId: ROUTE_SESSION_ID });

    expect(result.current.effectiveSessionId).toBe(ROUTE_SESSION_ID);
  });
});

describe("useSessionLayoutState fallback sessions", () => {
  it("uses the provided session when no active session exists", () => {
    const { result } = renderLayoutHook({ sessionId: ROUTE_SESSION_ID });

    expect(result.current.effectiveSessionId).toBe(ROUTE_SESSION_ID);
  });

  it("returns null without an active or provided session", () => {
    const { result } = renderLayoutHook();

    expect(result.current.effectiveSessionId).toBeNull();
  });
});

describe("useSessionLayoutState plan badge", () => {
  it("loads an existing plan before deriving the unseen mobile badge", async () => {
    const plan = makePlan();
    planApiMocks.getTaskPlan.mockResolvedValueOnce(plan);

    const { result } = renderLayoutHook();

    await waitFor(() => expect(result.current.plan).toEqual(plan));
    expect(planApiMocks.getTaskPlan).toHaveBeenCalledWith(ACTIVE_TASK_ID);
    expect(result.current.hasUnseenPlanUpdate).toBe(true);
  });
});
