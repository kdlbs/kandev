import { createElement, type ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { TaskPlan } from "@/lib/types/http-agents";
import type { TaskSession } from "@/lib/types/http";

const mockSetTaskPlan = vi.fn();
const mockSetActiveSession = vi.fn();
const mockToast = vi.fn();
const mockWsRequest = vi.fn();
const mockSetChatDraftContent = vi.fn();
const mockMarkPlanImplementationStarted = vi.fn();
const mockLaunchSession = vi.fn();

let mockStoreState: {
  taskSessions: { items: Record<string, TaskSession> };
  setActiveSession: typeof mockSetActiveSession;
  setTaskPlan: typeof mockSetTaskPlan;
} = {
  taskSessions: { items: {} },
  setActiveSession: mockSetActiveSession,
  setTaskPlan: mockSetTaskPlan,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof mockStoreState) => unknown) => selector(mockStoreState),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockWsRequest }),
}));

vi.mock("@/lib/local-storage", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    setChatDraftContent: (...args: unknown[]) => mockSetChatDraftContent(...args),
  };
});

vi.mock("@/lib/api/domains/plan-api", () => ({
  markPlanImplementationStarted: (...args: unknown[]) => mockMarkPlanImplementationStarted(...args),
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => mockLaunchSession(...args),
}));

vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: {
    getState: () => ({ filesBySessionId: {} }),
  },
}));

import { useImplementPlanRunner } from "./use-plan-actions";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";

function makePlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: TASK_ID,
    title: "Plan",
    content: "## Plan",
    created_by: "agent",
    created_at: "",
    updated_at: "",
    implementation_started_at: "2026-07-09T12:00:00Z",
    implementation_started_session_id: SESSION_ID,
    implementation_started_by: "user",
    ...overrides,
  };
}

function makeChatRef(value = "ship it") {
  const clear = vi.fn();
  return {
    ref: {
      current: {
        focusInput: vi.fn(),
        getTextareaElement: () => null,
        getValue: () => value,
        getSelectionStart: () => 0,
        insertText: vi.fn(),
        getAttachments: () => [],
        clear,
      },
    },
    clear,
  };
}

function queryWrapper(queryClient: QueryClient) {
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function renderImplementPlanRunner(args: Parameters<typeof useImplementPlanRunner>[0]) {
  const queryClient = makeQueryClient();
  return {
    queryClient,
    ...renderHook(() => useImplementPlanRunner(args), { wrapper: queryWrapper(queryClient) }),
  };
}

function setup() {
  vi.clearAllMocks();
  mockStoreState = {
    taskSessions: { items: {} },
    setActiveSession: mockSetActiveSession,
    setTaskPlan: mockSetTaskPlan,
  };
  mockWsRequest.mockResolvedValue({ success: true });
  mockMarkPlanImplementationStarted.mockResolvedValue(makePlan());
}

describe("useImplementPlanRunner same-session path", () => {
  beforeEach(() => setup());

  it("sends the implementation prompt, marks the plan, and clears plan mode", async () => {
    const plan = makePlan();
    mockMarkPlanImplementationStarted.mockResolvedValueOnce(plan);
    const handlePlanModeChange = vi.fn();
    const { ref, clear } = makeChatRef();
    const { result, queryClient } = renderImplementPlanRunner({
      resolvedSessionId: SESSION_ID,
      taskId: TASK_ID,
      handlePlanModeChange,
      chatInputRef: ref,
    });

    let ok = false;
    await act(async () => {
      ok = await result.current(false);
    });

    expect(ok).toBe(true);
    expect(mockWsRequest).toHaveBeenNthCalledWith(
      1,
      "message.add",
      expect.objectContaining({
        task_id: TASK_ID,
        session_id: SESSION_ID,
        content: expect.stringContaining("ship it"),
        plan_mode: false,
      }),
      10000,
    );
    expect(mockMarkPlanImplementationStarted).toHaveBeenCalledWith(TASK_ID, SESSION_ID);
    expect(queryClient.getQueryData(qk.taskPlan.detail(TASK_ID))).toBe(plan);
    expect(handlePlanModeChange).toHaveBeenCalledWith(false);
    expect(clear).toHaveBeenCalledTimes(1);
    expect(mockSetChatDraftContent).toHaveBeenCalledWith(SESSION_ID, null);
    expect(mockWsRequest).toHaveBeenNthCalledWith(
      2,
      "session.set_plan_mode",
      { session_id: SESSION_ID, enabled: false },
      5000,
    );
  });

  it("still clears plan mode when the durable marker write fails after send", async () => {
    mockMarkPlanImplementationStarted.mockRejectedValueOnce(new Error("marker offline"));
    const handlePlanModeChange = vi.fn();
    const { ref, clear } = makeChatRef();
    const { result } = renderImplementPlanRunner({
      resolvedSessionId: SESSION_ID,
      taskId: TASK_ID,
      handlePlanModeChange,
      chatInputRef: ref,
    });

    let ok = false;
    await act(async () => {
      ok = await result.current(false);
    });

    expect(ok).toBe(true);
    expect(handlePlanModeChange).toHaveBeenCalledWith(false);
    expect(clear).toHaveBeenCalledTimes(1);
    expect(mockToast).not.toHaveBeenCalled();
    expect(mockWsRequest).toHaveBeenCalledWith(
      "session.set_plan_mode",
      { session_id: SESSION_ID, enabled: false },
      5000,
    );
  });

  it("returns false and preserves composer state when sending fails", async () => {
    mockWsRequest.mockRejectedValueOnce(new Error("send failed"));
    const handlePlanModeChange = vi.fn();
    const { ref, clear } = makeChatRef();
    const { result } = renderImplementPlanRunner({
      resolvedSessionId: SESSION_ID,
      taskId: TASK_ID,
      handlePlanModeChange,
      chatInputRef: ref,
    });

    let ok = true;
    await act(async () => {
      ok = await result.current(false);
    });

    expect(ok).toBe(false);
    expect(mockMarkPlanImplementationStarted).not.toHaveBeenCalled();
    expect(handlePlanModeChange).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });
});

describe("useImplementPlanRunner toolbar path", () => {
  beforeEach(() => setup());

  it("can keep plan mode open for toolbar implementation", async () => {
    const plan = makePlan();
    mockMarkPlanImplementationStarted.mockResolvedValueOnce(plan);
    const handlePlanModeChange = vi.fn();
    const { result, queryClient } = renderImplementPlanRunner({
      resolvedSessionId: SESSION_ID,
      taskId: TASK_ID,
      handlePlanModeChange,
      clearPlanModeAfterSend: false,
    });

    let ok = false;
    await act(async () => {
      ok = await result.current(false);
    });

    expect(ok).toBe(true);
    expect(mockMarkPlanImplementationStarted).toHaveBeenCalledWith(TASK_ID, SESSION_ID);
    expect(queryClient.getQueryData(qk.taskPlan.detail(TASK_ID))).toBe(plan);
    expect(handlePlanModeChange).not.toHaveBeenCalled();
    expect(mockSetChatDraftContent).not.toHaveBeenCalled();
    expect(mockWsRequest).toHaveBeenCalledTimes(1);
  });
});
