import { describe, expect, it } from "vitest";
import { resolveResponsiveRightWidth } from "./right-width";

describe("resolveResponsiveRightWidth", () => {
  it("prefers a manual width when it fits within the cap", () => {
    expect(resolveResponsiveRightWidth(1600, 0, 320, 500)).toBe(320);
  });

  it("clamps a manual width to an explicit cap", () => {
    expect(resolveResponsiveRightWidth(1600, 0, 900, 500)).toBe(500);
  });

  it("falls back to the viewport when the dockview width is unavailable", () => {
    expect(resolveResponsiveRightWidth(undefined, 0, null, 500)).toBe(
      resolveResponsiveRightWidth(window.innerWidth, 0, null, 500),
    );
  });

  it("uses a supplied cap without recomputing the runtime cap", () => {
    expect(resolveResponsiveRightWidth(1600, 0, null, 280)).toBe(280);
  });
});
