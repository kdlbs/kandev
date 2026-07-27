import { describe, expect, it } from "vitest";
import { PULL_TO_REFRESH_THRESHOLD, pullDistance, shouldRefreshAfterPull } from "./pull-to-refresh";

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
});
