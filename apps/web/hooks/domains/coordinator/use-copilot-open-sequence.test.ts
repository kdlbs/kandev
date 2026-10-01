import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Coordinator, ConversationResponse } from "@/lib/api/domains/coordinator-api";

const mocks = vi.hoisted(() => ({
  getCoordinator: vi.fn(),
  openConversation: vi.fn(),
}));

vi.mock("@/lib/api/domains/coordinator-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/coordinator-api")>(
    "@/lib/api/domains/coordinator-api",
  );
  return {
    ...actual,
    getCoordinator: mocks.getCoordinator,
    openConversation: mocks.openConversation,
  };
});

import { runOpenSequence, useCopilotOpenSequence } from "./use-copilot-open-sequence";

const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "coord-1";

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: COORDINATOR_ID,
    workspace_id: WORKSPACE_ID,
    name: "Backend coordinator",
    agent_profile_id: "agent-1",
    executor_profile_id: "executor-1",
    task_agent_profile_id: "ap-1",
    task_executor_profile_id: "ep-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-28T00:00:00Z",
    updated_at: "2026-09-28T00:00:00Z",
    agent_profile_status: "ok",
    executor_profile_status: "ok",
    ...overrides,
  };
}

const conversation: ConversationResponse = {
  task_id: "task-1",
  session_id: "session-1",
  archive_state: false,
};

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

describe("runOpenSequence outcome table", () => {
  it("shows profile messages and never calls the route when a GET status is not ok", async () => {
    mocks.getCoordinator.mockResolvedValue(
      coordinator({ agent_profile_status: "missing", executor_profile_status: "ok" }),
    );

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({
      kind: "profile-unavailable",
      agentStatus: "missing",
      executorStatus: "ok",
    });
    expect(mocks.openConversation).not.toHaveBeenCalled();
  });

  it("shows the gone message on a GET 404", async () => {
    mocks.getCoordinator.mockRejectedValue(new ApiError("not found", 404, { error: "not found" }));

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "gone" });
    expect(mocks.openConversation).not.toHaveBeenCalled();
  });

  it("shows the gone message on a route 404", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockRejectedValue(
      new ApiError("not found", 404, { error: "not found" }),
    );

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "gone" });
  });

  it("shows a load-failed error with Try again on a GET 500", async () => {
    mocks.getCoordinator.mockRejectedValue(new ApiError("boom", 500, { error: "boom" }));

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "error", error: "load-failed" });
  });

  it("shows a load-failed error on a GET network failure", async () => {
    mocks.getCoordinator.mockRejectedValue(new TypeError("network down"));

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "error", error: "load-failed" });
  });

  it("resolves to the session on route 200", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockResolvedValue(conversation);

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "ready", session: conversation });
  });

  it("shows profile messages from the route's 409 coordinator_profile_unavailable body", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockRejectedValue(
      new ApiError("unavailable", 409, {
        error: "coordinator_profile_unavailable",
        agent_profile_status: "passthrough",
        executor_profile_status: "missing",
      }),
    );

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({
      kind: "profile-unavailable",
      agentStatus: "passthrough",
      executorStatus: "missing",
    });
  });

  it("shows a conflict error with Try again on route 409 conversation_conflict", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockRejectedValue(
      new ApiError("conflict", 409, {
        error: "conversation_conflict",
        error_code: "conversation_conflict",
      }),
    );

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "error", error: "conflict" });
  });

  it("shows an open-failed error with Try again on route 502", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockRejectedValue(new ApiError("bad gateway", 502, { error: "boom" }));

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "error", error: "open-failed" });
  });

  it("shows an open-failed error on a route network failure", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockRejectedValue(new TypeError("network down"));

    const result = await runOpenSequence(WORKSPACE_ID, COORDINATOR_ID);

    expect(result).toEqual({ kind: "error", error: "open-failed" });
  });
});

describe("useCopilotOpenSequence", () => {
  it("starts idle and moves to loading then ready across an open() call", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockResolvedValue(conversation);

    const { result } = renderHook(() => useCopilotOpenSequence(WORKSPACE_ID, COORDINATOR_ID));
    expect(result.current.state).toEqual({ kind: "idle" });

    act(() => result.current.open());
    expect(result.current.state).toEqual({ kind: "loading" });

    await waitFor(() =>
      expect(result.current.state).toEqual({ kind: "ready", session: conversation }),
    );
  });

  it("issues exactly one GET and one route call when two opens race", async () => {
    let resolveGet: ((value: Coordinator) => void) | undefined;
    mocks.getCoordinator.mockImplementation(
      () =>
        new Promise<Coordinator>((resolve) => {
          resolveGet = resolve;
        }),
    );
    mocks.openConversation.mockResolvedValue(conversation);

    const { result } = renderHook(() => useCopilotOpenSequence(WORKSPACE_ID, COORDINATOR_ID));
    act(() => result.current.open());
    act(() => result.current.open());

    expect(mocks.getCoordinator).toHaveBeenCalledTimes(1);
    resolveGet?.(coordinator());
    await waitFor(() => expect(result.current.state.kind).toBe("ready"));
    expect(mocks.openConversation).toHaveBeenCalledTimes(1);
  });

  it("discards a response that resolves after the viewed coordinator changed", async () => {
    let resolveGet: ((value: Coordinator) => void) | undefined;
    mocks.getCoordinator.mockImplementation(
      () =>
        new Promise<Coordinator>((resolve) => {
          resolveGet = resolve;
        }),
    );

    const { result, rerender } = renderHook(
      ({ coordinatorId }: { coordinatorId: string }) =>
        useCopilotOpenSequence(WORKSPACE_ID, coordinatorId),
      { initialProps: { coordinatorId: "coord-a" } },
    );
    act(() => result.current.open());
    expect(result.current.state).toEqual({ kind: "loading" });

    rerender({ coordinatorId: "coord-b" });
    expect(result.current.state).toEqual({ kind: "idle" });

    resolveGet?.(coordinator({ id: "coord-a" }));
    await Promise.resolve();
    await Promise.resolve();

    expect(result.current.state).toEqual({ kind: "idle" });
  });

  it("retry re-runs the GET and then the route exactly once", async () => {
    mocks.getCoordinator
      .mockRejectedValueOnce(new ApiError("boom", 500, { error: "boom" }))
      .mockResolvedValueOnce(coordinator());
    mocks.openConversation.mockResolvedValue(conversation);

    const { result } = renderHook(() => useCopilotOpenSequence(WORKSPACE_ID, COORDINATOR_ID));
    act(() => result.current.open());
    await waitFor(() =>
      expect(result.current.state).toEqual({ kind: "error", error: "load-failed" }),
    );

    act(() => result.current.retry());
    await waitFor(() =>
      expect(result.current.state).toEqual({ kind: "ready", session: conversation }),
    );

    expect(mocks.getCoordinator).toHaveBeenCalledTimes(2);
    expect(mocks.openConversation).toHaveBeenCalledTimes(1);
  });

  it("re-runs the sequence on every open, even once already ready", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockResolvedValue(conversation);

    const { result } = renderHook(() => useCopilotOpenSequence(WORKSPACE_ID, COORDINATOR_ID));
    act(() => result.current.open());
    await waitFor(() => expect(result.current.state.kind).toBe("ready"));

    act(() => result.current.open());
    await waitFor(() => expect(mocks.getCoordinator).toHaveBeenCalledTimes(2));
    expect(mocks.openConversation).toHaveBeenCalledTimes(2);
  });
});

// Review round 4 Finding D2 / TS-002: a consumer needs to tell whether its
// own open()/retry() call is what produced the current `state`, as opposed
// to a call that joined an already in-flight sequence and never started an
// attempt of its own. `settledAttemptId` names the attempt that produced the
// current `state`; `open()`/`retry()` return the id of the attempt they
// actually started, or `null` when they joined one.
describe("useCopilotOpenSequence settledAttemptId attribution", () => {
  it("returns null when a second open joins an in-flight call, and settledAttemptId reports only the joined attempt", async () => {
    let resolveGet: ((value: Coordinator) => void) | undefined;
    mocks.getCoordinator.mockImplementation(
      () =>
        new Promise<Coordinator>((resolve) => {
          resolveGet = resolve;
        }),
    );
    mocks.openConversation.mockResolvedValue(conversation);

    const { result } = renderHook(() => useCopilotOpenSequence(WORKSPACE_ID, COORDINATOR_ID));

    let firstAttempt: number | null = null;
    let secondAttempt: number | null = null;
    act(() => {
      firstAttempt = result.current.open();
    });
    act(() => {
      secondAttempt = result.current.open();
    });

    expect(typeof firstAttempt).toBe("number");
    expect(secondAttempt).toBeNull();
    expect(result.current.settledAttemptId).toBeNull();

    resolveGet?.(coordinator());
    await waitFor(() => expect(result.current.state.kind).toBe("ready"));
    expect(result.current.settledAttemptId).toBe(firstAttempt);
  });

  it("gives each non-overlapping open() call a distinct attempt id matching settledAttemptId at resolution", async () => {
    mocks.getCoordinator.mockResolvedValue(coordinator());
    mocks.openConversation.mockResolvedValue(conversation);

    const { result } = renderHook(() => useCopilotOpenSequence(WORKSPACE_ID, COORDINATOR_ID));

    let firstAttempt: number | null = null;
    act(() => {
      firstAttempt = result.current.open();
    });
    await waitFor(() => expect(result.current.settledAttemptId).toBe(firstAttempt));

    let secondAttempt: number | null = null;
    act(() => {
      secondAttempt = result.current.open();
    });
    await waitFor(() => expect(result.current.settledAttemptId).toBe(secondAttempt));
    expect(secondAttempt).not.toBeNull();
    expect(secondAttempt).not.toBe(firstAttempt);
  });
});
