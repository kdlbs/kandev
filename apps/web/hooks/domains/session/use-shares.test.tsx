import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

const listSharesMock = vi.fn();

vi.mock("@/lib/api/domains/share-api", () => ({
  listShares: (...args: unknown[]) => listSharesMock(...args),
}));

import { useShares } from "./use-shares";

beforeEach(() => {
  listSharesMock.mockReset();
});

describe("useShares", () => {
  it("loads shares on mount and exposes them", async () => {
    listSharesMock.mockResolvedValueOnce({
      shares: [
        {
          id: "s-1",
          url: "https://gist.github.com/u/a",
          created_at: "2026-05-21T12:00:00.000Z",
          snapshot_size_bytes: 100,
        },
      ],
    });
    const { result } = renderHook(() => useShares("t-1", "sess-1"));

    // Wait on the actual async outcome (shares populated) rather than
    // isLoading flipping false, which can be true on the initial render
    // before the effect resolves.
    await waitFor(() => expect(result.current.shares).toHaveLength(1));
    expect(result.current.shares[0]?.id).toBe("s-1");
    expect(listSharesMock).toHaveBeenCalledWith("t-1", "sess-1");
  });

  it("does not call listShares when ids are missing", async () => {
    const { result } = renderHook(() => useShares(null, "sess-1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(listSharesMock).not.toHaveBeenCalled();
    expect(result.current.shares).toEqual([]);
  });

  it("captures errors from the API", async () => {
    listSharesMock.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useShares("t-1", "sess-1"));
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.error?.message).toBe("boom");
  });

  it("refresh() re-fetches and updates state", async () => {
    listSharesMock.mockResolvedValueOnce({ shares: [] }).mockResolvedValueOnce({
      shares: [
        {
          id: "s-1",
          url: "u",
          created_at: "now",
          snapshot_size_bytes: 1,
        },
      ],
    });
    const { result } = renderHook(() => useShares("t-1", "sess-1"));
    await waitFor(() => expect(listSharesMock).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.shares).toHaveLength(1);
    expect(listSharesMock).toHaveBeenCalledTimes(2);
  });
});
