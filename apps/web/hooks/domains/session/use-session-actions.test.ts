import { describe, it, expect, vi, beforeEach } from "vitest";
import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { TaskSession } from "@/lib/types/http";
import {
  isSessionStoppable,
  isSessionDeletable,
  isSessionResumable,
  useSessionActions,
} from "./use-session-actions";

let queryClient: QueryClient;

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client: queryClient }, children);
}

/** Seed the by-task session list cache the delete path reads from. */
function seedByTask(taskId: string, sessions: Array<{ id: string; started_at: string }>): void {
  queryClient.setQueryData(qk.taskSession.byTask(taskId), {
    sessions: sessions as unknown as TaskSession[],
    total: sessions.length,
  });
}

const mockToast = vi.fn().mockReturnValue("toast-1");
const mockUpdateToast = vi.fn();
const mockRequest = vi.fn();
const mockSetActiveSessionAuto = vi.fn();
const mockClearActiveSession = vi.fn();

const T1 = "2025-01-01T00:00:00Z";
const T2 = "2025-01-02T00:00:00Z";
const T3 = "2025-01-03T00:00:00Z";

let mockState: Record<string, unknown> = {};

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: mockUpdateToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => mockState,
  }),
}));

describe("session state predicates", () => {
  it("isSessionStoppable returns true for active states", () => {
    expect(isSessionStoppable("RUNNING")).toBe(true);
    expect(isSessionStoppable("STARTING")).toBe(true);
    expect(isSessionStoppable("WAITING_FOR_INPUT")).toBe(true);
    expect(isSessionStoppable("COMPLETED")).toBe(false);
    expect(isSessionStoppable("FAILED")).toBe(false);
  });

  it("isSessionDeletable returns false for in-flight states", () => {
    expect(isSessionDeletable("RUNNING")).toBe(false);
    expect(isSessionDeletable("STARTING")).toBe(false);
    expect(isSessionDeletable("WAITING_FOR_INPUT")).toBe(true);
    expect(isSessionDeletable("COMPLETED")).toBe(true);
    expect(isSessionDeletable("FAILED")).toBe(true);
  });

  it("isSessionResumable returns true for terminal states", () => {
    expect(isSessionResumable("COMPLETED")).toBe(true);
    expect(isSessionResumable("FAILED")).toBe(true);
    expect(isSessionResumable("CANCELLED")).toBe(true);
    expect(isSessionResumable("RUNNING")).toBe(false);
    expect(isSessionResumable("STARTING")).toBe(false);
  });
});

function resetActionsTest(): void {
  vi.clearAllMocks();
  mockToast.mockReturnValue("toast-1");
  mockRequest.mockResolvedValue(undefined);
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  mockState = {
    tasks: { activeSessionId: null },
    setActiveSessionAuto: mockSetActiveSessionAuto,
    clearActiveSession: mockClearActiveSession,
  };
}

describe("useSessionActions — dispatch", () => {
  beforeEach(resetActionsTest);

  it("setPrimary dispatches session.set_primary with session id", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }), {
      wrapper,
    });
    await result.current.setPrimary();
    expect(mockRequest).toHaveBeenCalledWith("session.set_primary", { session_id: "s1" }, 15000);
  });

  it("stop dispatches session.stop", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }), {
      wrapper,
    });
    await result.current.stop();
    expect(mockRequest).toHaveBeenCalledWith("session.stop", { session_id: "s1" }, 15000);
  });

  it("resume dispatches session.launch with intent=resume and 30s timeout", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }), {
      wrapper,
    });
    await result.current.resume();
    expect(mockRequest).toHaveBeenCalledWith(
      "session.launch",
      { task_id: "t1", intent: "resume", session_id: "s1" },
      30000,
    );
  });

  it("actions no-op when sessionId is missing", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: null, taskId: "t1" }), {
      wrapper,
    });
    await result.current.setPrimary();
    await result.current.stop();
    await result.current.remove();
    expect(mockRequest).not.toHaveBeenCalled();
  });
});

describe("useSessionActions — remove", () => {
  beforeEach(resetActionsTest);

  it("remove deletes via WS, removes from the TQ cache, and runs onDeleted callback", async () => {
    seedByTask("t1", [
      { id: "s1", started_at: T1 },
      { id: "s2", started_at: T2 },
    ]);
    const onDeleted = vi.fn();
    const { result } = renderHook(
      () => useSessionActions({ sessionId: "s1", taskId: "t1", onDeleted }),
      { wrapper },
    );
    await result.current.remove();
    await waitFor(() => expect(onDeleted).toHaveBeenCalled());
    expect(mockRequest).toHaveBeenCalledWith("session.delete", { session_id: "s1" }, 15000);
    const remaining = queryClient.getQueryData<{ sessions: TaskSession[] }>(
      qk.taskSession.byTask("t1"),
    );
    expect(remaining?.sessions.map((s) => s.id)).toEqual(["s2"]);
  });

  it("remove no-ops when WS request fails (cache untouched)", async () => {
    seedByTask("t1", [{ id: "s1", started_at: T1 }]);
    mockRequest.mockRejectedValueOnce(new Error("network down"));
    const onDeleted = vi.fn();
    const { result } = renderHook(
      () => useSessionActions({ sessionId: "s1", taskId: "t1", onDeleted }),
      { wrapper },
    );
    await result.current.remove();
    const remaining = queryClient.getQueryData<{ sessions: TaskSession[] }>(
      qk.taskSession.byTask("t1"),
    );
    expect(remaining?.sessions.map((s) => s.id)).toEqual(["s1"]);
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("remove hands off to most-recent remaining session when active was deleted", async () => {
    mockState = {
      tasks: { activeSessionId: "s1" },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
    seedByTask("t1", [
      { id: "s1", started_at: T1 },
      { id: "s2", started_at: T2 },
      { id: "s3", started_at: T3 },
    ]);
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }), {
      wrapper,
    });
    await result.current.remove();
    expect(mockSetActiveSessionAuto).toHaveBeenCalledWith("t1", "s3");
    expect(mockClearActiveSession).not.toHaveBeenCalled();
  });

  it("remove clears active session when no other sessions remain", async () => {
    mockState = {
      tasks: { activeSessionId: "s1" },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
    seedByTask("t1", [{ id: "s1", started_at: T1 }]);
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }), {
      wrapper,
    });
    await result.current.remove();
    expect(mockClearActiveSession).toHaveBeenCalled();
    expect(mockSetActiveSessionAuto).not.toHaveBeenCalled();
  });
});
