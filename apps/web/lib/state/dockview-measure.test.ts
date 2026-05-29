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
