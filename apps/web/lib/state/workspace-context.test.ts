import { describe, expect, it } from "vitest";
import { retryAfterMilliseconds } from "./workspace-context";

describe("retryAfterMilliseconds", () => {
  it("caps a server delay at the browser timer limit", () => {
    expect(retryAfterMilliseconds({ retryAfterSeconds: 3_000_000 })).toBe(2_147_483_647);
  });

  it("rejects non-positive and non-finite delays", () => {
    expect(retryAfterMilliseconds({ retryAfterSeconds: 0 })).toBeUndefined();
    expect(retryAfterMilliseconds({ retryAfterSeconds: Number.POSITIVE_INFINITY })).toBeUndefined();
  });
});
