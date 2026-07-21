import { describe, expect, it } from "vitest";
import { isCurrentIdentityRequest } from "./request-identity";

describe("isCurrentIdentityRequest", () => {
  it("rejects responses from an earlier generation or MR identity", () => {
    expect(isCurrentIdentityRequest(2, 2, "ws/a/1", "ws/a/1")).toBe(true);
    expect(isCurrentIdentityRequest(1, 2, "ws/a/1", "ws/a/1")).toBe(false);
    expect(isCurrentIdentityRequest(2, 2, "ws/a/1", "ws/b/1")).toBe(false);
  });
});
