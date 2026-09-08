import { describe, expect, it } from "vitest";
import { isPublicWebhook } from "./plugin-manifest-model";

describe("isPublicWebhook", () => {
  it("preserves the v1 public default", () => {
    expect(isPublicWebhook(1, undefined)).toBe(true);
  });

  it("uses the v2 authenticated default", () => {
    expect(isPublicWebhook(2, undefined)).toBe(false);
  });

  it("honors explicit access in every version", () => {
    expect(isPublicWebhook(1, "authenticated")).toBe(false);
    expect(isPublicWebhook(2, "public")).toBe(true);
  });
});
