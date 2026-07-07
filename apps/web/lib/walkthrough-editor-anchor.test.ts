import { describe, expect, it } from "vitest";
import {
  computeWalkthroughConnectorPath,
  type WalkthroughViewportRect,
} from "./walkthrough-editor-anchor";

function rect(overrides: Partial<WalkthroughViewportRect>): WalkthroughViewportRect {
  const left = overrides.left ?? 0;
  const top = overrides.top ?? 0;
  const width = overrides.width ?? 100;
  const height = overrides.height ?? 40;
  return {
    left,
    top,
    width,
    height,
    right: overrides.right ?? left + width,
    bottom: overrides.bottom ?? top + height,
  };
}

describe("computeWalkthroughConnectorPath", () => {
  it("draws from the nearest card side to the walkthrough range", () => {
    const path = computeWalkthroughConnectorPath(
      rect({ left: 700, top: 80, width: 300, height: 220 }),
      rect({ left: 100, top: 180, width: 500, height: 36 }),
    );

    expect(path).toMatch(/^M 700\.0 198\.0 C /);
    expect(path).toContain("600.0 198.0");
  });

  it("returns null for empty geometry", () => {
    expect(computeWalkthroughConnectorPath(rect({ width: 0 }), rect({}))).toBeNull();
  });
});
