import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { useControlRoutingCircuits } from "./use-control-circuits";

const api = vi.hoisted(() => ({ read: vi.fn() }));
vi.mock("@/lib/api/domains/routing-circuits-api", () => ({ getRoutingCircuits: api.read }));
const circuits = [
  {
    resource_key: "profile:one",
    scope: "profile",
    scope_value: "one",
    state: "open",
    code: "rate_limited",
    failures: 1,
  },
];
const revision = () => ({ owner: {}, identity: "authorized" });
beforeEach(() => api.read.mockReset());
afterEach(cleanup);

describe("control circuit read ownership", () => {
  it("does not read without an authorized snapshot", () => {
    const { result } = renderHook(() => useControlRoutingCircuits(undefined));
    expect(api.read).not.toHaveBeenCalled();
    expect(result.current.status).toBe("loading");
  });

  it("retains same-owner data after a temporary read failure", async () => {
    api.read.mockResolvedValueOnce({ circuits }).mockRejectedValueOnce(new Error("offline"));
    const initial = revision();
    const { result, rerender } = renderHook(({ snapshot }) => useControlRoutingCircuits(snapshot), {
      initialProps: { snapshot: initial },
    });
    await waitFor(() => expect(result.current.status).toBe("ready"));
    const receivedAt = result.current.receivedAt;
    rerender({ snapshot: { ...initial } });
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.data).toEqual(circuits);
    expect(result.current.stale).toBe(true);
    expect(result.current.receivedAt).toBe(receivedAt);
  });

  it.each([403, 404])(
    "discards retained data after denied or unsupported response %s",
    async (status) => {
      api.read
        .mockResolvedValueOnce({ circuits })
        .mockRejectedValueOnce(new ApiError("unavailable", status, {}));
      const initial = revision();
      const { result, rerender } = renderHook(
        ({ snapshot }) => useControlRoutingCircuits(snapshot),
        { initialProps: { snapshot: initial } },
      );
      await waitFor(() => expect(result.current.status).toBe("ready"));
      rerender({ snapshot: { ...initial } });
      await waitFor(() =>
        expect(result.current.status).toBe(status === 403 ? "denied" : "unsupported"),
      );
      expect(result.current.data).toBeUndefined();
    },
  );

  it("ignores an old owner's deferred response", async () => {
    let finish!: (value: unknown) => void;
    api.read
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      )
      .mockResolvedValueOnce({ circuits: [] });
    const { result, rerender } = renderHook(({ snapshot }) => useControlRoutingCircuits(snapshot), {
      initialProps: { snapshot: revision() },
    });
    rerender({ snapshot: revision() });
    await waitFor(() => expect(result.current.status).toBe("ready"));
    await act(async () => finish({ circuits }));
    expect(result.current.data).toEqual([]);
  });
});
