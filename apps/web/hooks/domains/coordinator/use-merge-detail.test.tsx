import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MergeProposal } from "@/lib/api/domains/coordinator-api";

const getMock = vi.fn();
const checkMock = vi.fn();
const releaseMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-merge-api", () => ({
  getMergeProposal: (...a: unknown[]) => getMock(...a),
  checkMergeNow: (...a: unknown[]) => checkMock(...a),
  releaseMerge: (...a: unknown[]) => releaseMock(...a),
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));

import { useMergeDetail } from "./use-merge-detail";

function proposal(over: Partial<MergeProposal> = {}): MergeProposal {
  return {
    id: "p1",
    coordinator_id: "c1",
    workspace_id: "w1",
    status: "uncertain",
    kind: "merge",
    spec: {
      task_id: "t1",
      repository_id: "r1",
      change_key: "github|h|r1|4",
      head_sha: "abc",
      method: "squash",
    },
    final_spec: null,
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as unknown as MergeProposal;
}

const detail = { change_key: "k", dispatch: { can_check: true, can_release: false } };

beforeEach(() => vi.clearAllMocks());

describe("useMergeDetail", () => {
  it("reads the detail on open", async () => {
    getMock.mockResolvedValue({ ...proposal(), merge: detail });
    const { result } = renderHook(() => useMergeDetail("w1", "c1", proposal()));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.detail).toEqual(detail);
  });

  it("reports a failed read and re-reads on demand", async () => {
    getMock.mockRejectedValueOnce(new Error("provider"));
    const { result } = renderHook(() => useMergeDetail("w1", "c1", proposal()));
    await waitFor(() => expect(result.current.status).toBe("error"));
    getMock.mockResolvedValue({ ...proposal(), merge: detail });
    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.status).toBe("ready");
  });

  it("sends one check for a double press", async () => {
    getMock.mockResolvedValue({ ...proposal(), merge: detail });
    let resolve!: (p: MergeProposal) => void;
    checkMock.mockReturnValue(new Promise<MergeProposal>((r) => (resolve = r)));
    const { result } = renderHook(() => useMergeDetail("w1", "c1", proposal()));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => {
      void result.current.checkNow();
      void result.current.checkNow();
    });
    expect(checkMock).toHaveBeenCalledTimes(1);
    await act(async () => resolve(proposal({ status: "approved" })));
    await waitFor(() => expect(result.current.busy).toBe(false));
  });

  it("records a failed release", async () => {
    getMock.mockResolvedValue({ ...proposal(), merge: detail });
    releaseMock.mockRejectedValue(new Error("no"));
    const { result } = renderHook(() => useMergeDetail("w1", "c1", proposal()));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => {
      await result.current.release();
    });
    expect(result.current.actionError).toBe("release");
  });
});
