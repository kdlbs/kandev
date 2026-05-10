import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  isSessionStoppable,
  isSessionDeletable,
  isSessionResumable,
  useSessionActions,
} from "./use-session-actions";

const mockToast = vi.fn().mockReturnValue("toast-1");
const mockUpdateToast = vi.fn();
const mockRequest = vi.fn();
const mockRemoveTaskSession = vi.fn();
const mockSetActiveSessionAuto = vi.fn();
const mockClearActiveSession = vi.fn();

let mockState: Record<string, unknown> = {};

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: mockUpdateToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      removeTaskSession: mockRemoveTaskSession,
    }),
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

describe("useSessionActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockToast.mockReturnValue("toast-1");
    mockRequest.mockResolvedValue(undefined);
    mockState = {
      tasks: { activeSessionId: null },
      taskSessionsByTask: { itemsByTaskId: {} },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
  });

  it("setPrimary dispatches session.set_primary with session id", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.setPrimary();
    expect(mockRequest).toHaveBeenCalledWith("session.set_primary", { session_id: "s1" }, 15000);
  });

  it("stop dispatches session.stop", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.stop();
    expect(mockRequest).toHaveBeenCalledWith("session.stop", { session_id: "s1" }, 15000);
  });

  it("resume dispatches session.launch with intent=resume and 30s timeout", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.resume();
    expect(mockRequest).toHaveBeenCalledWith(
      "session.launch",
      { task_id: "t1", intent: "resume", session_id: "s1" },
      30000,
    );
  });

  it("remove deletes via WS, removes from store, and runs onDeleted callback", async () => {
    const onDeleted = vi.fn();
    const { result } = renderHook(() =>
      useSessionActions({ sessionId: "s1", taskId: "t1", onDeleted }),
    );
    await result.current.remove();
    await waitFor(() => expect(onDeleted).toHaveBeenCalled());
    expect(mockRequest).toHaveBeenCalledWith("session.delete", { session_id: "s1" }, 15000);
    expect(mockRemoveTaskSession).toHaveBeenCalledWith("t1", "s1");
  });

  it("remove no-ops when WS request fails (store untouched)", async () => {
    mockRequest.mockRejectedValueOnce(new Error("network down"));
    const onDeleted = vi.fn();
    const { result } = renderHook(() =>
      useSessionActions({ sessionId: "s1", taskId: "t1", onDeleted }),
    );
    await result.current.remove();
    expect(mockRemoveTaskSession).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("remove hands off to most-recent remaining session when active was deleted", async () => {
    mockState = {
      tasks: { activeSessionId: "s1" },
      taskSessionsByTask: {
        itemsByTaskId: {
          t1: [
            { id: "s1", started_at: "2025-01-01T00:00:00Z" },
            { id: "s2", started_at: "2025-01-02T00:00:00Z" },
            { id: "s3", started_at: "2025-01-03T00:00:00Z" },
          ],
        },
      },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.remove();
    expect(mockSetActiveSessionAuto).toHaveBeenCalledWith("t1", "s3");
    expect(mockClearActiveSession).not.toHaveBeenCalled();
  });

  it("remove clears active session when no other sessions remain", async () => {
    mockState = {
      tasks: { activeSessionId: "s1" },
      taskSessionsByTask: {
        itemsByTaskId: { t1: [{ id: "s1", started_at: "2025-01-01T00:00:00Z" }] },
      },
      setActiveSessionAuto: mockSetActiveSessionAuto,
      clearActiveSession: mockClearActiveSession,
    };
    const { result } = renderHook(() => useSessionActions({ sessionId: "s1", taskId: "t1" }));
    await result.current.remove();
    expect(mockClearActiveSession).toHaveBeenCalled();
    expect(mockSetActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("actions no-op when sessionId is missing", async () => {
    const { result } = renderHook(() => useSessionActions({ sessionId: null, taskId: "t1" }));
    await result.current.setPrimary();
    await result.current.stop();
    await result.current.remove();
    expect(mockRequest).not.toHaveBeenCalled();
  });
});
