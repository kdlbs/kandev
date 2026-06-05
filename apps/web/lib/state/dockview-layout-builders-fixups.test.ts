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
  isDebug: () => false,
}));

vi.mock("@/lib/local-storage", () => ({
  getGlobalSidebarWidth: vi.fn(() => null),
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
  getPinnedWidth: vi.fn(() => 350),
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
  getPinnedWidth,
  RIGHT_TOP_GROUP,
} from "./layout-manager";
import { getGlobalSidebarWidth } from "@/lib/local-storage";

const SIDEBAR_GROUP = "group-sidebar";
const CENTER_GROUP = "group-center";
const RIGHT_BOTTOM_GROUP = "group-right-bottom";

let mockResizeView: ReturnType<typeof vi.fn>;

function mockSplitview(sizesByIndex: number[]): void {
  mockResizeView = vi.fn();
  vi.mocked(getRootSplitview).mockReturnValue({
    length: sizesByIndex.length,
    getViewSize: (idx: number) => sizesByIndex[idx],
    resizeView: mockResizeView,
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
    vi.mocked(getGlobalSidebarWidth).mockReturnValue(null);
    vi.mocked(getPinnedWidth).mockReturnValue(350);
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

  it("uses the default sidebar target instead of an env-saved live width when no global pref exists", () => {
    // Slow fromJSON restore can bring back an env-specific sidebar width. With
    // sidebar now a global pref, no pref means "use the default", not the
    // env-saved live width.
    mockSplitview([420, 1050]);
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("sidebar", 350);
  });

  it("clamps a global sidebar pref down to the cap", () => {
    vi.mocked(getGlobalSidebarWidth).mockReturnValue(900);
    mockSplitview([474, 996]);
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("sidebar", SIDEBAR_CAP);
  });

  it("uses the column default for the pinned right column when no saved width is given", () => {
    // Regression (dockview-wrong-width): the default-preset right column must
    // anchor to a STABLE width — the per-env saved width, or the column default
    // when none — NOT dockview's post-fromJSON live size (400 here). Capturing
    // the live size ratcheted the column wider on every restore. Mirrors
    // captureSidebarTarget, which anchors to the pref/default, never the live
    // size. getPinnedWidth is mocked to 350 (the default).
    mockSplitview([350, 720, 400]); // live right = 400 (transient/rescaled)
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]);

    applyLayoutFixups(api);

    expect(setPinnedTarget).toHaveBeenCalledWith("sidebar", 350);
    expect(setPinnedTarget).toHaveBeenCalledWith("right", 350); // default, NOT live 400
    expect(setPinnedTarget).not.toHaveBeenCalledWith("right", 400);
  });

  it("anchors the pinned right target to the saved per-env width, not the live size", () => {
    // A restore passes the env's saved right width; it wins over both the live
    // size (400) and the default (350) so a deliberately-resized task restores
    // its own remembered width. Also asserts that the column is physically
    // resized to the stable width (not left at the transient live size).
    mockSplitview([350, 720, 400]);
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP]);

    applyLayoutFixups(api, 420);

    expect(setPinnedTarget).toHaveBeenCalledWith("right", 420);
    expect(setPinnedTarget).not.toHaveBeenCalledWith("right", 400);
    expect(mockResizeView).toHaveBeenCalledWith(2, 420);
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

  it("clamps an over-cap saved right width down to the cap", () => {
    mockSplitview([350, 200, 800]);
    const api = makeApi([SIDEBAR_GROUP, CENTER_GROUP, RIGHT_TOP_GROUP]);

    applyLayoutFixups(api, 1200); // saved 1200 exceeds RIGHT_CAP (1029)

    expect(setPinnedTarget).toHaveBeenCalledWith("right", RIGHT_CAP);
  });
});
