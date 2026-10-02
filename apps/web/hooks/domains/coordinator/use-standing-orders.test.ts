import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";

const listMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return { ...actual, listStandingOrders: (...args: unknown[]) => listMock(...args) };
});

import { useStandingOrders } from "./use-standing-orders";

function order(id: string, number: number): StandingOrder {
  return {
    id,
    number,
    text: `Rule ${number}`,
    created_at: "2026-09-12T00:00:00Z",
    created_by: "u-1",
    retired_at: null,
    last_applied_at: null,
  };
}

beforeEach(() => listMock.mockReset());

describe("useStandingOrders", () => {
  it("lists the active orders", async () => {
    listMock.mockResolvedValueOnce({ orders: [order("o1", 1), order("o2", 2)] });
    const { result } = renderHook(() => useStandingOrders("ws-1", "c-1"));
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.orders.map((o) => o.id)).toEqual(["o1", "o2"]);
    expect(listMock).toHaveBeenCalledWith("ws-1", "c-1", { includeRetired: false });
  });

  it("reads retired orders too when asked", async () => {
    listMock.mockResolvedValueOnce({ orders: [] });
    const { result } = renderHook(() => useStandingOrders("ws-1", "c-1", { includeRetired: true }));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(listMock).toHaveBeenCalledWith("ws-1", "c-1", { includeRetired: true });
  });

  it("reports an error and recovers on retry", async () => {
    listMock.mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useStandingOrders("ws-1", "c-1"));
    await waitFor(() => expect(result.current.status).toBe("error"));
    listMock.mockResolvedValueOnce({ orders: [] });
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.orders).toEqual([]);
  });

  it("keeps the newest list when an older reload lands late", async () => {
    let resolveOld: (value: { orders: StandingOrder[] }) => void = () => {};
    listMock
      .mockReturnValueOnce(new Promise((resolve) => (resolveOld = resolve)))
      .mockResolvedValueOnce({ orders: [order("new", 1)] });
    const { result } = renderHook(() => useStandingOrders("ws-1", "c-1"));
    act(() => {
      void result.current.reload();
    });
    await waitFor(() => expect(result.current.orders[0]?.id).toBe("new"));
    await act(async () => resolveOld({ orders: [order("old", 1)] }));
    expect(result.current.orders[0]?.id).toBe("new");
  });

  it("keeps the list on a failed reload", async () => {
    listMock.mockResolvedValueOnce({ orders: [order("o1", 1)] });
    const { result } = renderHook(() => useStandingOrders("ws-1", "c-1"));
    await waitFor(() => expect(result.current.status).toBe("ready"));
    listMock.mockRejectedValueOnce(new Error("boom"));
    act(() => {
      void result.current.reload();
    });
    await waitFor(() => expect(listMock).toHaveBeenCalledTimes(2));
    expect(result.current.status).toBe("ready");
    expect(result.current.orders).toHaveLength(1);
  });
});
