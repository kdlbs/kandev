import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const getMock = vi.fn();
const reviewMock = vi.fn();
let wsHandler: ((m: { payload: Record<string, string> }) => void) | null = null;

vi.mock("@/lib/api/domains/coordinator-automatic-api", () => ({
  getClassEligibility: (...a: unknown[]) => getMock(...a),
  recordClassReview: (...a: unknown[]) => reviewMock(...a),
}));
vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => ({
    on: (_: string, h: typeof wsHandler) => {
      wsHandler = h;
      return () => {
        wsHandler = null;
      };
    },
  }),
}));

import { useClassEligibility } from "./use-class-eligibility";

const eligibility = (eligible: boolean) => ({
  eligible,
  conditions: [],
  setting: "requires_approval",
  changed_by: "",
  changed_at: null,
});

beforeEach(() => {
  getMock.mockReset();
  reviewMock.mockReset();
  wsHandler = null;
});

describe("useClassEligibility", () => {
  it("reads nothing while disabled", () => {
    renderHook(() => useClassEligibility("w", "c", "create_task", false));
    expect(getMock).not.toHaveBeenCalled();
  });

  it("loads eligibility and reports an error status on failure", async () => {
    getMock.mockResolvedValueOnce(eligibility(true));
    const ok = renderHook(() => useClassEligibility("w", "c", "create_task", true));
    await waitFor(() => expect(ok.result.current.status).toBe("ready"));
    expect(ok.result.current.eligibility?.eligible).toBe(true);

    getMock.mockRejectedValueOnce(new Error("boom"));
    const bad = renderHook(() => useClassEligibility("w", "c", "create_task", true));
    await waitFor(() => expect(bad.result.current.status).toBe("error"));
  });

  it("re-reads on a matching coordinator.updated event only", async () => {
    getMock.mockResolvedValue(eligibility(false));
    const { result } = renderHook(() => useClassEligibility("w", "c", "create_task", true));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    getMock.mockClear();
    act(() => wsHandler?.({ payload: { workspace_id: "w", coordinator_id: "other" } }));
    expect(getMock).not.toHaveBeenCalled();
    act(() => wsHandler?.({ payload: { workspace_id: "w", coordinator_id: "c" } }));
    expect(getMock).toHaveBeenCalledTimes(1);
  });

  it("marks reviewed then re-reads, and flags a failed review", async () => {
    getMock.mockResolvedValue(eligibility(false));
    reviewMock.mockResolvedValueOnce({});
    const { result } = renderHook(() => useClassEligibility("w", "c", "create_task", true));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    getMock.mockClear();
    await act(() => result.current.markReviewed());
    expect(reviewMock).toHaveBeenCalledWith("w", "c", "create_task");
    expect(getMock).toHaveBeenCalledTimes(1);
    expect(result.current.reviewFailed).toBe(false);

    reviewMock.mockRejectedValueOnce(new Error("503"));
    await act(() => result.current.markReviewed());
    expect(result.current.reviewFailed).toBe(true);
  });
});
