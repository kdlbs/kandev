import { describe, expect, it } from "vitest";
import {
  isAtVerticalScrollTop,
  PULL_TO_REFRESH_THRESHOLD,
  pullDistance,
  shouldRefreshAfterPull,
} from "./pull-to-refresh";

describe("pull-to-refresh gesture", () => {
  it("dampens pull distance and caps it at the refresh threshold", () => {
    expect(pullDistance(-20)).toBe(0);
    expect(pullDistance(80)).toBe(44);
    expect(pullDistance(500)).toBe(PULL_TO_REFRESH_THRESHOLD);
  });

  it("refreshes only after the threshold is reached", () => {
    expect(shouldRefreshAfterPull(PULL_TO_REFRESH_THRESHOLD - 1)).toBe(false);
    expect(shouldRefreshAfterPull(PULL_TO_REFRESH_THRESHOLD)).toBe(true);
  });

  it("recognizes the touched scroll owner and nested ancestors", () => {
    const root = document.createElement("div");
    const scrollOwner = document.createElement("div");
    const target = document.createElement("button");
    root.append(scrollOwner);
    scrollOwner.append(target);
    Object.defineProperties(scrollOwner, {
      clientHeight: { configurable: true, value: 100 },
      scrollHeight: { configurable: true, value: 200 },
      scrollTop: { configurable: true, value: 0 },
    });
    expect(isAtVerticalScrollTop(target, root)).toBe(true);
    Object.defineProperty(scrollOwner, "scrollTop", { configurable: true, value: 10 });
    expect(isAtVerticalScrollTop(target, root)).toBe(false);
  });
});
