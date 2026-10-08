import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { TaskSession } from "@/lib/types/http";
import { sessionId as toSessionId } from "@/lib/types/ids";

const mocks = vi.hoisted(() => ({
  getCoordinator: vi.fn(),
  openConversation: vi.fn(),
  useTaskSessions: vi.fn(),
  useSession: vi.fn(),
}));

vi.mock("@/lib/api/domains/coordinator-api", () => ({
  getCoordinator: mocks.getCoordinator,
  openConversation: mocks.openConversation,
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: mocks.useTaskSessions,
}));

vi.mock("@/hooks/domains/session/use-session", () => ({
  useSession: mocks.useSession,
}));

import { useCoordinatorLauncher } from "./use-coordinator-launcher";

const WORKSPACE_ID = "ws-1";

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "coord-1",
    workspace_id: WORKSPACE_ID,
    name: "Backend coordinator",
    agent_profile_id: "agent-1",
    executor_profile_id: "executor-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-28T00:00:00Z",
    updated_at: "2026-09-28T00:00:00Z",
    ...overrides,
  };
}

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: toSessionId("session-1"),
    task_id: "task-1",
    state: "IDLE",
    ...overrides,
  } as TaskSession;
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.useTaskSessions.mockReturnValue({
    sessions: [],
    isLoading: false,
    isLoaded: true,
    error: null,
    loadSessions: vi.fn(),
  });
  mocks.useSession.mockReturnValue({ session: null, isActive: false, isFailed: false });
});

afterEach(cleanup);

describe("useCoordinatorLauncher", () => {
  it("issues the coordinator GET and never the conversation route", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());

    renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", null));

    await waitFor(() => expect(mocks.getCoordinator).toHaveBeenCalledWith(WORKSPACE_ID, "coord-1"));
    expect(mocks.openConversation).not.toHaveBeenCalled();
  });

  it("shows not busy and reads no session when conversation_task_id is null", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator({ conversation_task_id: null }));

    const { result } = renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", null));

    await waitFor(() => expect(result.current.coordinator).not.toBeNull());
    expect(result.current.busy).toBe(false);
    expect(mocks.useTaskSessions).toHaveBeenLastCalledWith(null);
    expect(mocks.useSession).toHaveBeenLastCalledWith(null);
  });

  it("is busy when the resolved session is RUNNING", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator({ conversation_task_id: "task-1" }));
    mocks.useTaskSessions.mockReturnValue({
      sessions: [session({ id: toSessionId("session-1"), is_primary: true, state: "RUNNING" })],
      isLoading: false,
      isLoaded: true,
      error: null,
      loadSessions: vi.fn(),
    });
    mocks.useSession.mockReturnValue({
      session: session({ id: toSessionId("session-1"), state: "RUNNING" }),
      isActive: true,
      isFailed: false,
    });

    const { result } = renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", null));

    await waitFor(() => expect(result.current.busy).toBe(true));
  });

  it("is busy when foreground_activity is background", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator({ conversation_task_id: "task-1" }));
    mocks.useTaskSessions.mockReturnValue({
      sessions: [
        session({ id: toSessionId("session-1"), is_primary: true, state: "WAITING_FOR_INPUT" }),
      ],
      isLoading: false,
      isLoaded: true,
      error: null,
      loadSessions: vi.fn(),
    });
    mocks.useSession.mockReturnValue({
      session: session({
        id: toSessionId("session-1"),
        state: "WAITING_FOR_INPUT",
        foreground_activity: "background",
      }),
      isActive: true,
      isFailed: false,
    });

    const { result } = renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", null));

    await waitFor(() => expect(result.current.busy).toBe(true));
  });

  it("prefers the latest route session id over the GET's conversation_task_id", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator({ conversation_task_id: "task-1" }));

    renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", "route-session-1"));

    await waitFor(() => expect(mocks.useSession).toHaveBeenLastCalledWith("route-session-1"));
    expect(mocks.useTaskSessions).toHaveBeenLastCalledWith(null);
  });

  it("flips busy when the subscribed session's state changes while the popover is closed", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator({ conversation_task_id: "task-1" }));
    mocks.useTaskSessions.mockReturnValue({
      sessions: [session({ id: toSessionId("session-1"), is_primary: true, state: "IDLE" })],
      isLoading: false,
      isLoaded: true,
      error: null,
      loadSessions: vi.fn(),
    });
    mocks.useSession.mockReturnValue({
      session: session({ id: toSessionId("session-1"), state: "IDLE" }),
      isActive: true,
      isFailed: false,
    });

    const { result, rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string }) =>
        useCoordinatorLauncher(WORKSPACE_ID, coordinatorId, null),
      { initialProps: { coordinatorId: "coord-1" } },
    );
    await waitFor(() => expect(result.current.coordinator).not.toBeNull());
    expect(result.current.busy).toBe(false);

    mocks.useSession.mockReturnValue({
      session: session({ id: toSessionId("session-1"), state: "RUNNING" }),
      isActive: true,
      isFailed: false,
    });
    rerender({ coordinatorId: "coord-1" });

    expect(result.current.busy).toBe(true);
  });
});

describe("useCoordinatorLauncher - gone and coordinator switching", () => {
  it("reports gone on a 404 and shows not busy", async () => {
    mocks.getCoordinator.mockRejectedValue(new ApiError("not found", 404, { error: "not found" }));

    const { result } = renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", null));

    await waitFor(() => expect(result.current.gone).toBe(true));
    expect(result.current.busy).toBe(false);
    expect(result.current.coordinator).toBeNull();
  });

  it("does not report gone on a non-404 failure", async () => {
    mocks.getCoordinator.mockRejectedValue(new ApiError("boom", 500, { error: "boom" }));

    const { result } = renderHook(() => useCoordinatorLauncher(WORKSPACE_ID, "coord-1", null));

    await waitFor(() => expect(mocks.getCoordinator).toHaveBeenCalled());
    expect(result.current.gone).toBe(false);
  });

  it("resets gone when the viewed coordinator changes", async () => {
    mocks.getCoordinator.mockRejectedValueOnce(
      new ApiError("not found", 404, { error: "not found" }),
    );
    mocks.getCoordinator.mockResolvedValueOnce(coordinator({ id: "coord-2" }));

    const { result, rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string }) =>
        useCoordinatorLauncher(WORKSPACE_ID, coordinatorId, null),
      { initialProps: { coordinatorId: "coord-1" } },
    );
    await waitFor(() => expect(result.current.gone).toBe(true));

    rerender({ coordinatorId: "coord-2" });
    await waitFor(() => expect(result.current.coordinator?.id).toBe("coord-2"));
    expect(result.current.gone).toBe(false);
  });

  it("follows the newly viewed coordinator when the old one's GET resolves later", async () => {
    let resolveA: ((value: Coordinator) => void) | undefined;
    const coordA = coordinator({ id: "coord-a", conversation_task_id: null });
    const coordB = coordinator({ id: "coord-b", conversation_task_id: null });
    mocks.getCoordinator.mockImplementation((_workspaceId: string, coordinatorId: string) => {
      if (coordinatorId === "coord-a") {
        return new Promise<Coordinator>((resolve) => {
          resolveA = resolve;
        });
      }
      return Promise.resolve(coordB);
    });

    const { result, rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string }) =>
        useCoordinatorLauncher(WORKSPACE_ID, coordinatorId, null),
      { initialProps: { coordinatorId: "coord-a" } },
    );

    rerender({ coordinatorId: "coord-b" });
    await waitFor(() => expect(result.current.coordinator?.id).toBe("coord-b"));

    resolveA?.(coordA);
    await Promise.resolve();
    await Promise.resolve();

    expect(result.current.coordinator?.id).toBe("coord-b");
  });
});
