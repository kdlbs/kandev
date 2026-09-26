import { describe, expect, it } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useLatestOnly } from "./use-latest-only";

// Regression for the Office task-detail refetch race: a WS trigger can
// fire again before a prior GET resolves, and a slower earlier response
// can arrive after a faster later one. useLatestOnly must let the caller
// discard the stale response regardless of arrival order.
describe("useLatestOnly", () => {
  it("marks only the most recently begun token as current", () => {
    const { result } = renderHook(() => useLatestOnly());

    let tokenA = -1;
    let tokenB = -1;
    act(() => {
      tokenA = result.current.begin();
      tokenB = result.current.begin();
    });

    expect(result.current.isCurrent(tokenA)).toBe(false);
    expect(result.current.isCurrent(tokenB)).toBe(true);
  });

  it("discards an earlier call's result when it resolves after a later call started", async () => {
    const { result } = renderHook(() => useLatestOnly());
    const applied: string[] = [];

    // Simulates the exact race: call A starts, call B starts before A
    // resolves, then A's (stale) response arrives followed by B's.
    const tokenA = result.current.begin();
    const tokenB = result.current.begin();

    if (result.current.isCurrent(tokenA)) applied.push("A");
    if (result.current.isCurrent(tokenB)) applied.push("B");

    expect(applied).toEqual(["B"]);
  });

  it("accepts a call's result when no newer call has started since", () => {
    const { result } = renderHook(() => useLatestOnly());

    const token = result.current.begin();

    expect(result.current.isCurrent(token)).toBe(true);
  });
});
