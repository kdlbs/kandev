import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";

vi.mock("@/lib/local-storage", () => ({
  getManualRightWidth: vi.fn(() => null),
}));

vi.mock("./dockview-measure", () => ({
  measureDockviewGridWidth: vi.fn(() => 800),
}));

import { getManualRightWidth } from "@/lib/local-storage";
import { enforcePinnedTargets } from "./dockview-pinned-enforce";
import { clearAllPinnedTargets, setPinnedTarget } from "./layout-manager";

function makeApi(orientation?: "VERTICAL" | "HORIZONTAL") {
  const splitview = {
    orientation,
    length: 2,
    getViewSize: vi.fn(() => 400),
    resizeView: vi.fn(),
  };
  const api = {
    component: { gridview: { root: { splitview } } },
    hasMaximizedGroup: vi.fn(() => false),
  } as unknown as DockviewApi;
  return { api, splitview };
}

describe("enforcePinnedTargets", () => {
  beforeEach(() => {
    clearAllPinnedTargets();
    vi.mocked(getManualRightWidth).mockReturnValue(null);
  });

  it("clamps an oversized manual right width to the current viewport cap", () => {
    vi.mocked(getManualRightWidth).mockReturnValue(600);
    const { api, splitview } = makeApi();

    enforcePinnedTargets(api, {
      sidebarVisible: false,
      rightPanelsVisible: true,
      maximized: false,
      envId: "env-a",
    });

    expect(splitview.resizeView).toHaveBeenCalledWith(1, 320);
  });

  it("clamps an oversized pinned right width to the current viewport cap", () => {
    setPinnedTarget("right", 600);
    const { api, splitview } = makeApi();

    enforcePinnedTargets(api, {
      sidebarVisible: false,
      rightPanelsVisible: true,
      maximized: false,
      envId: "env-a",
    });

    expect(splitview.resizeView).toHaveBeenCalledWith(1, 320);
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("enforcePinnedTargets root orientation", () => {
  const ctx = {
    sidebarVisible: true,
    rightPanelsVisible: true,
    maximized: false,
    envId: "env-a",
  };

  beforeEach(() => {
    clearAllPinnedTargets();
    vi.mocked(getManualRightWidth).mockReturnValue(null);
  });

  it("leaves the rows of a VERTICAL root alone even with right and sidebar targets", () => {
    setPinnedTarget("right", 250);
    setPinnedTarget("sidebar", 260);
    vi.mocked(getManualRightWidth).mockReturnValue(240);
    const { api, splitview } = makeApi("VERTICAL");

    enforcePinnedTargets(api, ctx);

    expect(splitview.resizeView).not.toHaveBeenCalled();
  });

  it.each([["HORIZONTAL" as const], [undefined]])(
    "still restores the right column of a %s root",
    (orientation) => {
      setPinnedTarget("right", 250);
      const { api, splitview } = makeApi(orientation);

      enforcePinnedTargets(api, ctx);

      expect(splitview.resizeView).toHaveBeenCalledWith(1, 250);
    },
  );
});
