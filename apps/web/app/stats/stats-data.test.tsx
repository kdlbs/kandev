import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchGlobalStats: vi.fn(),
  fetchTaskStats: vi.fn(),
  fetchDailyActivity: vi.fn(),
  fetchCompletedActivity: vi.fn(),
  fetchModelUsage: vi.fn(),
  fetchRepositoryStats: vi.fn(),
  fetchGitStats: vi.fn(),
}));

vi.mock("@/lib/api/domains/stats-api", () => mocks);

import { composeStatsResponse, useStatsSections } from "./stats-data";

function configureSuccessfulResponses() {
  mocks.fetchGlobalStats.mockResolvedValue({
    total_tasks: 1,
    completed_tasks: 1,
    in_progress_tasks: 0,
    total_sessions: 1,
    total_turns: 1,
    total_messages: 1,
    total_user_messages: 1,
    total_tool_calls: 0,
    total_duration_ms: 1000,
    avg_turns_per_task: 1,
    avg_messages_per_task: 1,
    avg_duration_ms_per_task: 1000,
    avg_turn_duration_ms: 1000,
    avg_messages_per_turn: 1,
  });
  mocks.fetchTaskStats.mockResolvedValue({ task_stats: [], task_stats_has_more: false });
  mocks.fetchDailyActivity.mockResolvedValue([]);
  mocks.fetchCompletedActivity.mockResolvedValue([]);
  mocks.fetchModelUsage.mockResolvedValue([]);
  mocks.fetchRepositoryStats.mockResolvedValue([]);
  mocks.fetchGitStats.mockResolvedValue({
    total_commits: 0,
    total_files_changed: 0,
    total_insertions: 0,
    total_deletions: 0,
  });
}

async function flushRequests() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("useStatsSections", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    configureSuccessfulResponses();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("recovers a transient failed section", async () => {
    vi.useFakeTimers();
    mocks.fetchDailyActivity
      .mockRejectedValueOnce(Object.assign(new Error("busy"), { status: 503 }))
      .mockResolvedValueOnce([]);

    const { result } = renderHook(() => useStatsSections("ws-1", "month"));
    await flushRequests();
    expect(result.current.daily.kind).toBe("error");
    expect(result.current.global.kind).toBe("ready");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });
    expect(mocks.fetchDailyActivity).toHaveBeenCalledTimes(2);
    expect(result.current.daily.kind).toBe("ready");
  });

  it("stops after the retry budget", async () => {
    vi.useFakeTimers();
    mocks.fetchDailyActivity.mockRejectedValue(new Error("offline"));

    const { result } = renderHook(() => useStatsSections("ws-1", "month"));
    await flushRequests();
    expect(result.current.daily.kind).toBe("error");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    expect(mocks.fetchDailyActivity).toHaveBeenCalledTimes(3);
  });

  it("cancels retries on range and workspace change", async () => {
    vi.useFakeTimers();
    mocks.fetchDailyActivity.mockRejectedValue(new Error("offline"));
    const { result, rerender } = renderHook(
      ({ workspaceId, range }: { workspaceId: string; range: "month" | "week" }) =>
        useStatsSections(workspaceId, range),
      { initialProps: { workspaceId: "ws-1", range: "month" as "month" | "week" } },
    );
    await flushRequests();
    expect(result.current.daily.kind).toBe("error");

    rerender({ workspaceId: "ws-2", range: "week" });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    const workspaces = mocks.fetchDailyActivity.mock.calls.map(([id]) => id);
    expect(workspaces.filter((id) => id === "ws-1")).toHaveLength(1);
    expect(workspaces.at(-1)).toBe("ws-2");
    expect(mocks.fetchDailyActivity).toHaveBeenCalledWith(
      "ws-2",
      expect.objectContaining({ init: expect.anything() }),
      "week",
    );
  });

  it("coalesces recovery triggers while a section request is active", async () => {
    mocks.fetchDailyActivity.mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(() => useStatsSections("ws-1", "month"));
    await flushRequests();
    const deferred = new Promise<[]>(() => {});
    mocks.fetchDailyActivity.mockReturnValue(deferred);

    act(() => {
      result.current.retrySection("daily");
      result.current.retrySection("daily");
    });

    expect(mocks.fetchDailyActivity).toHaveBeenCalledTimes(2);
  });

  it("does not automatically retry permanent or authorization failures", async () => {
    vi.useFakeTimers();
    mocks.fetchDailyActivity.mockRejectedValue({ status: 403 });
    const { result } = renderHook(() => useStatsSections("ws-1", "month"));
    await flushRequests();
    expect(result.current.daily.kind).toBe("error");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(mocks.fetchDailyActivity).toHaveBeenCalledTimes(1);
  });

  it("gates Copy Stats until every current-selection section is ready", async () => {
    const { result } = renderHook(() => useStatsSections("ws-1", "month"));
    await waitFor(() => expect(result.current.daily.kind).toBe("ready"));
    expect(composeStatsResponse(result.current)).not.toBeNull();

    mocks.fetchDailyActivity.mockRejectedValueOnce(new Error("offline"));
    const { result: changed } = renderHook(() => useStatsSections("ws-1", "week"));
    await flushRequests();
    expect(composeStatsResponse(changed.current)).toBeNull();
  });
});
