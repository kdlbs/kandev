import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockList = vi.fn();
vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomationSummaries: (...args: unknown[]) => mockList(...args),
}));

import { useAutomationSummaries } from "./use-automation-summaries";
import type { AutomationSummary } from "@/lib/types/automation";

const WORKSPACE = "ws-1";
const A1 = "a1";
const SOCKET_CLOSED = "socket closed";

function mkSummary(automationId: string, openRuns = 0): AutomationSummary {
  return { automation_id: automationId, open_runs: openRuns };
}

beforeEach(() => {
  mockList.mockResolvedValue([]);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useAutomationSummaries", () => {
  it("loads the workspace it was given", async () => {
    mockList.mockResolvedValue([mkSummary(A1, 2)]);

    const { result } = renderHook(() => useAutomationSummaries(WORKSPACE));

    await waitFor(() => expect(result.current.summaries).toHaveLength(1));
    expect(mockList).toHaveBeenCalledWith(WORKSPACE);
    expect(result.current.summaries[0].open_runs).toBe(2);
  });

  it("asks for nothing, and does not sit loading, without a workspace", async () => {
    const { result } = renderHook(() => useAutomationSummaries(undefined));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockList).not.toHaveBeenCalled();
    expect(result.current.summaries).toEqual([]);
  });

  it("shows nothing from the previous workspace while switching", async () => {
    // Health read under the wrong automations is worse than an empty list: the
    // rows would name this workspace's automations and describe another's.
    mockList.mockResolvedValue([mkSummary(A1)]);
    const { result, rerender } = renderHook(({ ws }) => useAutomationSummaries(ws), {
      initialProps: { ws: WORKSPACE },
    });
    await waitFor(() => expect(result.current.summaries).toHaveLength(1));

    let release: (value: unknown) => void = () => {};
    mockList.mockImplementation(() => new Promise((resolve) => (release = resolve)));
    rerender({ ws: "ws-2" });

    expect(result.current.summaries).toEqual([]);
    expect(result.current.loading).toBe(true);

    await act(async () => {
      release([mkSummary("a9")]);
    });
    await waitFor(() => expect(result.current.summaries[0]?.automation_id).toBe("a9"));
  });

  it("surfaces a failure instead of showing an empty workspace", async () => {
    mockList.mockRejectedValue(new Error(SOCKET_CLOSED));

    const { result } = renderHook(() => useAutomationSummaries(WORKSPACE));

    await waitFor(() => expect(result.current.error).toBe(SOCKET_CLOSED));
    expect(result.current.loading).toBe(false);
  });

  it("keeps the last successful summaries when a refresh fails", async () => {
    mockList
      .mockResolvedValueOnce([mkSummary(A1, 1)])
      .mockRejectedValueOnce(new Error(SOCKET_CLOSED));

    const { result } = renderHook(() => useAutomationSummaries(WORKSPACE));

    await waitFor(() => expect(result.current.summaries[0]?.open_runs).toBe(1));

    await act(async () => {
      result.current.refresh();
    });
    await waitFor(() => expect(result.current.error).toBe(SOCKET_CLOSED));

    expect(result.current.summaries).toEqual([mkSummary(A1, 1)]);
  });

  it("clears a previous workspace's error when switching", async () => {
    mockList.mockRejectedValueOnce(new Error(SOCKET_CLOSED));
    const { result, rerender } = renderHook(({ ws }) => useAutomationSummaries(ws), {
      initialProps: { ws: WORKSPACE },
    });
    await waitFor(() => expect(result.current.error).toBe(SOCKET_CLOSED));

    mockList.mockResolvedValue([mkSummary(A1)]);
    rerender({ ws: "ws-2" });

    expect(result.current.error).toBeNull();
  });

  it("ignores a slow response that lost its race", async () => {
    let releaseFirst: (value: unknown) => void = () => {};
    mockList.mockImplementationOnce(() => new Promise((resolve) => (releaseFirst = resolve)));
    const { result } = renderHook(() => useAutomationSummaries(WORKSPACE));

    mockList.mockResolvedValue([mkSummary("fresh")]);
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.summaries[0]?.automation_id).toBe("fresh"));

    await act(async () => {
      releaseFirst([mkSummary("stale")]);
    });

    expect(result.current.summaries[0]?.automation_id).toBe("fresh");
  });
});
