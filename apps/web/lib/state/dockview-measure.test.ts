import { afterEach, describe, expect, it } from "vitest";
import type { DockviewApi } from "dockview-react";
import { measureDockviewContainer } from "./dockview-measure";

function fakeApi(width: number, height: number): DockviewApi {
  return { width, height } as unknown as DockviewApi;
}

describe("measureDockviewContainer", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("uses the live container size when it is laid out", () => {
    const parent = document.createElement("div");
    Object.defineProperty(parent, "clientWidth", { value: 1500, configurable: true });
    Object.defineProperty(parent, "clientHeight", { value: 700, configurable: true });
    const dv = document.createElement("div");
    dv.className = "dv-dockview";
    parent.appendChild(dv);
    document.body.appendChild(parent);

    expect(measureDockviewContainer(fakeApi(0, 0))).toEqual({ width: 1500, height: 700 });
  });

  it("ignores an implausibly-narrow live width during root layout but keeps the live height", () => {
    // Regression (post unified-sidebar #1165): while the AppSidebar root-flex
    // width was animated, the dockview content container could be measured at a
    // tiny positive width. Keep rejecting that transient during initial layout:
    // building the default at it collapses the horizontal columns into a
    // vertical stack (chat / files+changes / terminal). On desktop the sidebar
    // maxes at 30vw, so content is always >= ~70vw — a sub-half-viewport read is
    // definitionally a transient: fall back the width to the viewport so the
    // build stays horizontal (the resize observer then snaps to the exact size).
    // Root flex layout changes are horizontal, so the live height is reliable.
    const parent = document.createElement("div");
    Object.defineProperty(parent, "clientWidth", { value: 80, configurable: true });
    Object.defineProperty(parent, "clientHeight", { value: 700, configurable: true });
    const dv = document.createElement("div");
    dv.className = "dv-dockview";
    parent.appendChild(dv);
    document.body.appendChild(parent);

    const result = measureDockviewContainer(fakeApi(0, 0));
    expect(result.width).toBe(window.innerWidth);
    expect(result.height).toBe(700);
  });

  it("never returns a zero size on a fresh mount (no container, api not laid out yet)", () => {
    // Regression: a 0×0 measurement builds the default layout at zero width, so
    // dockview collapses the horizontal columns into a vertical stack (chat /
    // files+changes / terminal). Fall back to the viewport instead so the
    // default builds horizontally; the resize observer then snaps it to the
    // exact container size.
    const { width, height } = measureDockviewContainer(fakeApi(0, 0));
    expect(width).toBeGreaterThan(0);
    expect(height).toBeGreaterThan(0);
    expect(width).toBe(window.innerWidth);
    expect(height).toBe(window.innerHeight);
  });
});
