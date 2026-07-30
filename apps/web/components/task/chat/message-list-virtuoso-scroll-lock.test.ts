import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { useGuardedFollowOutput } from "./message-list-virtuoso-scroll-lock";

describe("useGuardedFollowOutput", () => {
  it("follows smoothly at the bottom when enabled and unlocked", () => {
    const { result } = renderHook(() => useGuardedFollowOutput(() => false, true));
    expect(result.current(true)).toBe("smooth");
  });

  it("does not follow when away from the bottom", () => {
    const { result } = renderHook(() => useGuardedFollowOutput(() => false, true));
    expect(result.current(false)).toBe(false);
  });

  it("does not follow while a programmatic scroll is locked, even at the bottom", () => {
    const { result } = renderHook(() => useGuardedFollowOutput(() => true, true));
    expect(result.current(true)).toBe(false);
  });

  it("does not follow while auto-scroll is disabled, even at the bottom and unlocked", () => {
    const { result } = renderHook(() => useGuardedFollowOutput(() => false, false));
    expect(result.current(true)).toBe(false);
  });

  it("does not follow when both disabled and locked", () => {
    const { result } = renderHook(() => useGuardedFollowOutput(() => true, false));
    expect(result.current(true)).toBe(false);
  });
});
