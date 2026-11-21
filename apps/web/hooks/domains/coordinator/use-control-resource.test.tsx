import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { controlReadFailure, useControlResource } from "./use-control-resource";

const scope = vi.hoisted(() => ({ identity: "owner-one" }));
vi.mock("@/components/state-provider", () => ({ useAppStore: () => scope.identity }));

describe("control read boundaries", () => {
  beforeEach(() => {
    scope.identity = "owner-one";
  });
  it.each([
    [401, "denied"],
    [403, "denied"],
    [404, "unsupported"],
    [501, "unsupported"],
    [500, "error"],
  ])("classifies HTTP %s without substituting zero", (status, expected) => {
    expect(controlReadFailure(new ApiError("failure", Number(status), null))).toBe(expected);
  });
  it("discards the previous identity synchronously and rejects a late read", async () => {
    const pending: Array<(value: string) => void> = [];
    const signals: AbortSignal[] = [];
    const read = (signal: AbortSignal) => {
      signals.push(signal);
      return new Promise<string>((resolve) => pending.push(resolve));
    };
    const revision = {};
    const { result, rerender } = renderHook(() => useControlResource(read, revision));
    await act(async () => {
      pending[0]("first owner");
    });
    expect(result.current.data).toBe("first owner");
    scope.identity = "owner-two";
    rerender();
    expect(result.current.data).toBeUndefined();
    expect(signals[0].aborted).toBe(true);
    await act(async () => {
      pending[1]("second owner");
    });
    await waitFor(() => expect(result.current.data).toBe("second owner"));
  });
  it("retains the same scope while refreshing, then clears it on denied", async () => {
    const read = vi
      .fn()
      .mockResolvedValueOnce([1])
      .mockRejectedValueOnce(new ApiError("denied", 403, null));
    const { result, rerender } = renderHook(
      ({ revision }) => useControlResource<number[]>(read, revision),
      { initialProps: { revision: {} } },
    );
    await waitFor(() => expect(result.current.data).toEqual([1]));
    rerender({ revision: {} });
    expect(result.current.data).toEqual([1]);
    expect(result.current.refreshing).toBe(true);
    await waitFor(() => expect(result.current.status).toBe("denied"));
    expect(result.current.data).toBeUndefined();
    expect(read).toHaveBeenCalledTimes(2);
  });
});

describe("control refresh continuity", () => {
  it("retains a dated snapshot on transient failure and recovers on reconnect", async () => {
    const read = vi
      .fn()
      .mockResolvedValueOnce([1])
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce([2]);
    const { result, rerender } = renderHook(
      ({ revision }) => useControlResource<number[]>(read, revision),
      { initialProps: { revision: {} } },
    );
    await waitFor(() => expect(result.current.data).toEqual([1]));
    const at = result.current.receivedAt;
    rerender({ revision: {} });
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.data).toEqual([1]);
    expect(result.current.receivedAt).toBe(at);
    expect(result.current.stale).toBe(true);
    rerender({ revision: {} });
    await waitFor(() => expect(result.current.data).toEqual([2]));
    expect(result.current.stale).toBeFalsy();
  });
  it("clears synchronously when a resource key changes or the route disables reads", async () => {
    const first = vi.fn().mockResolvedValue("first");
    const second = vi.fn().mockImplementation(() => new Promise(() => {}));
    const { result, rerender } = renderHook(
      ({ read, revision }: { read: typeof first; revision?: object }) =>
        useControlResource(read, revision),
      { initialProps: { read: first, revision: {} as object | undefined } },
    );
    await waitFor(() => expect(result.current.data).toBe("first"));
    rerender({ read: second, revision: {} });
    expect(result.current.data).toBeUndefined();
    rerender({ read: first, revision: undefined });
    expect(result.current.data).toBeUndefined();
  });
});
