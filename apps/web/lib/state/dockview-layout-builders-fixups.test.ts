import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";

// Dedicated file for `applyLayoutFixups` pinned-target capture. Mocks
// `./layout-manager` so we can drive the splitview geometry and spy on
// `setPinnedTarget` without standing up a real dockview instance. Kept
// separate from `dockview-layout-builders.test.ts` (which exercises
// `fallbackGroupPosition` against the real layout-manager constants).

const SIDEBAR_CAP = 441;
const RIGHT_CAP = 1029;

vi.mock("@/lib/debug/log", () => ({
  createDebugLogger: () => () => {},
  IS_DEBUG: false,
}));

vi.mock("./layout-manager", () => ({
  SIDEBAR_LOCK: "no-drop-target",
  SIDEBAR_GROUP: "group-sidebar",
  CENTER_GROUP: "group-center",
  RIGHT_TOP_GROUP: "group-right-top",
  RIGHT_BOTTOM_GROUP: "group-right-bottom",
  LAYOUT_PINNED_MIN_PX: 180,
  computeSidebarMaxPx: vi.fn(() => SIDEBAR_CAP),
  computeRightMaxPx: vi.fn(() => RIGHT_CAP),
  getRootSplitview: vi.fn(),
  resolveGroupIds: vi.fn(() => ({
    sidebarGroupId: "group-sidebar",
    centerGroupId: "group-center",
    rightTopGroupId: "group-right-top",
    rightBottomGroupId: "group-right-bottom",
  })),
  setPinnedTarget: vi.fn(),
}));

import { applyLayoutFixups } from "./dockview-layout-builders";
import {
  getRootSplitview,
  setPinnedTarget,
  computeSidebarMaxPx,
  computeRightMaxPx,
  RIGHT_TOP_GROUP,
} from "./layout-manager";

const SIDEBAR_GROUP = "group-sidebar";
const CENTER_GROUP = "group-center";
const RIGHT_BOTTOM_GROUP = "group-right-bottom";

function mockSplitview(sizesByIndex: number[]): void {
  vi.mocked(getRootSplitview).mockReturnValue({
    length: sizesByIndex.length,
    getViewSize: (idx: number) => sizesByIndex[idx],
  } as unknown as NonNullable<ReturnType<typeof getRootSplitview>>);
}

function makeApi(groupIds: string[]): DockviewApi {
  const sidebarGroup = {
    locked: undefined as unknown,
    header: { hidden: true },
    width: 0,
    api: { setConstraints: vi.fn() },
  };
  return {
    width: 1470,
    groups: groupIds.map((id) => ({ id, api: { setConstraints: vi.fn() } })),
    getPanel: (id: string) => (id === "sidebar" ? { group: sidebarGroup } : null),
  } as unknown as DockviewApi;
}

describe("applyLayoutFixups — pinned target capture", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does NOT record a right target in the 2-column fallback (center is last)", () => {
    // Regression: global-fallback restore strips env-scoped right panels,
    // leaving [sidebar, center]. The last splitview child is the CENTER
    // column — its width must never be recorded as the "right" target, or it
    // inflates the real right column and gets persisted into the env layout.
    mockSplitview([474, 996]); // idx0 = sidebar, idx1 = center (NOT right)
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP]); // no right groups

    applyLayoutFixups(api);

    expect(setPinnedTarget).not.toHaveBeenCalledWith("right", expect.anything());
  });

  it("clamps an over-cap sidebar target down to the cap", () => {
    // sidebar live (474) exceeds the cap (441); the constraint pins it at the
    // cap, so the target must be clamped or enforcePinnedTargets spins forever.
    mockSplitview([474, 996]);
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("sidebar", SIDEBAR_CAP);
  });

  it("records both targets in a real 3-column layout", () => {
    mockSplitview([350, 720, 400]); // sidebar, center, right
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("sidebar", 350);
    expect(setPinnedTarget).toHaveBeenCalledWith("right", 400);
  });

  it("derives the caps from api.width, not the window.innerWidth fallback", () => {
    // Regression: caps computed without the measured width fall back to
    // window.innerWidth, which can be transiently stale and clamp the captured
    // target too narrow. They must be derived from api.width (1470 here).
    mockSplitview([350, 720, 400]);
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP]);

    applyLayoutFixups(api);

    expect(computeSidebarMaxPx).toHaveBeenCalledWith(1470);
    expect(computeRightMaxPx).toHaveBeenCalledWith(1470);
  });

  it("records the side-column target for a 3-column preset without RIGHT_TOP_GROUP", () => {
    // Regression: vscode/preview/plan presets put their side column in a group
    // with a generated id (not RIGHT_TOP_GROUP). The target must still be
    // captured per-env, or switching to such a task leaks the previous task's
    // right target and snaps its side column to the wrong width.
    mockSplitview([350, 720, 420]); // sidebar, center, vscode/preview side col
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, "group-generated-7"]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("right", 420);
  });

  it("clamps an over-cap right target down to the cap", () => {
    mockSplitview([350, 200, 1200]); // right (1200) exceeds RIGHT_CAP (1029)
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("right", RIGHT_CAP);
  });
});
