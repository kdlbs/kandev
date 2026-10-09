import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GoalResponse } from "@/lib/api/domains/coordinator-api";

const getGoalMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return { ...actual, getGoal: (...args: unknown[]) => getGoalMock(...args) };
});

const ws = vi.hoisted(() => ({
  handlers: new Map<string, (message: { payload: Record<string, unknown> }) => void>(),
}));

vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => ({
    on: (type: string, handler: (message: { payload: Record<string, unknown> }) => void) => {
      ws.handlers.set(type, handler);
      return () => ws.handlers.delete(type);
    },
  }),
}));

import { useGoal } from "./use-goal";

const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "c-1";

function read(name: string | null): GoalResponse {
  return {
    active: name
      ? {
          id: "g-1",
          coordinator_id: COORDINATOR_ID,
          name,
          due_on: null,
          status: "active",
          criteria: [],
          baseline: null,
          set_at: "2026-09-20T00:00:00Z",
          met_at: null,
          met_by: null,
          created_at: "2026-09-20T00:00:00Z",
          updated_at: "2026-09-20T00:00:00Z",
        }
      : null,
    last_met: null,
    measures: null,
  };
}

beforeEach(() => {
  getGoalMock.mockReset();
  ws.handlers.clear();
});

describe("useGoal", () => {
  it("loads the goal read on mount", async () => {
    getGoalMock.mockResolvedValueOnce(read("Ship"));
    const { result } = renderHook(() => useGoal(WORKSPACE_ID, COORDINATOR_ID));
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.data?.active?.name).toBe("Ship");
  });

  it("refetches on coordinator.updated for this coordinator only", async () => {
    getGoalMock.mockResolvedValueOnce(read("Ship")).mockResolvedValueOnce(read("Ship 2"));
    const { result } = renderHook(() => useGoal(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(result.current.status).toBe("ready"));

    act(() =>
      ws.handlers.get("coordinator.updated")?.({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: "other" },
      }),
    );
    expect(getGoalMock).toHaveBeenCalledTimes(1);

    act(() =>
      ws.handlers.get("coordinator.updated")?.({
        payload: { workspace_id: WORKSPACE_ID, coordinator_id: COORDINATOR_ID },
      }),
    );
    await waitFor(() => expect(result.current.data?.active?.name).toBe("Ship 2"));
  });

  it("keeps the response of the last request sent when an older one lands late", async () => {
    let resolveOld: (value: GoalResponse) => void = () => {};
    getGoalMock
      .mockReturnValueOnce(new Promise<GoalResponse>((resolve) => (resolveOld = resolve)))
      .mockResolvedValueOnce(read("Newer"));
    const { result } = renderHook(() => useGoal(WORKSPACE_ID, COORDINATOR_ID));
    act(() => {
      result.current.reload();
    });
    await waitFor(() => expect(result.current.data?.active?.name).toBe("Newer"));
    await act(async () => resolveOld(read("Older")));
    expect(result.current.data?.active?.name).toBe("Newer");
  });

  it("reports an error only when there is nothing loaded, and keeps stale data on a failed refetch", async () => {
    getGoalMock.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useGoal(WORKSPACE_ID, COORDINATOR_ID));
    await waitFor(() => expect(result.current.status).toBe("error"));

    getGoalMock.mockResolvedValueOnce(read("Ship"));
    act(() => result.current.reload());
    await waitFor(() => expect(result.current.status).toBe("ready"));

    getGoalMock.mockRejectedValueOnce(new Error("boom"));
    act(() => result.current.reload());
    await waitFor(() => expect(getGoalMock).toHaveBeenCalledTimes(3));
    expect(result.current.status).toBe("ready");
    expect(result.current.data?.active?.name).toBe("Ship");
  });
});
