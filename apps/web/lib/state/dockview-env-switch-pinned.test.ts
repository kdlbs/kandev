import { describe, it, expect, vi, beforeEach } from "vitest";
import { performEnvSwitch, type EnvSwitchParams } from "./dockview-env-switch";

// Dedicated file for the post-fast-path pinned-column resize logic — kept
// separate from `dockview-env-switch.test.ts` so its mockReturnValue setups
// don't get contaminated by the larger suite's `mockReturnValueOnce` queues.

vi.mock("@/lib/local-storage", () => ({
  getEnvLayout: vi.fn(() => null),
}));

vi.mock("./dockview-layout-builders", () => ({
  applyLayoutFixups: vi.fn(() => ({
    sidebarGroupId: "g1",
    centerGroupId: "g2",
    rightTopGroupId: "g3",
    rightBottomGroupId: "g4",
  })),
}));

const sidebarRightColumns = [
  { id: "sidebar", pinned: true, groups: [] },
  { id: "center", groups: [] },
  { id: "right", pinned: true, groups: [] },
];

vi.mock("./layout-manager", () => {
  return {
    fromDockviewApi: vi.fn(() => ({ columns: sidebarRightColumns })),
    savedLayoutMatchesLive: vi.fn(() => false),
    layoutStructuresMatch: vi.fn(() => true),
    getRootSplitview: vi.fn(),
    // Distinct per-column defaults so we can prove the sidebar branch ignores
    // saved per-env sizes (it routes through getPinnedWidth, which honors the
    // global pref) while the right branch keeps using saved sizes.
    getPinnedWidth: vi.fn((col: { id: string }) => (col.id === "sidebar" ? 300 : 350)),
    // Used by applyPinnedColumnSizes and (indirectly) by the
    // formatWidthsSnapshot debug log it now emits.
    setPinnedTarget: vi.fn(),
    getPinnedTarget: vi.fn(() => undefined),
  };
});

import { getEnvLayout } from "@/lib/local-storage";
import { getRootSplitview, savedLayoutMatchesLive } from "./layout-manager";
import { applyLayoutFixups } from "./dockview-layout-builders";

function makeMockApi(): EnvSwitchParams["api"] {
  return {
    panels: [],
    groups: [],
    layout: vi.fn(),
    fromJSON: vi.fn(),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
  } as unknown as EnvSwitchParams["api"];
}

function makeParams(): EnvSwitchParams {
  return {
    api: makeMockApi(),
    oldEnvId: "old-env",
    newEnvId: "new-env",
    activeSessionId: "new-session",
    safeWidth: 800,
    safeHeight: 600,
    buildDefault: vi.fn(),
    getDefaultLayout: vi.fn(() => ({ columns: [] })),
  };
}

describe("performEnvSwitch — pinned column resize after fast-path", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("resizes sidebar and right to default widths when no saved layout exists", () => {
    // Regression: the fast path skips fromJSON, so column widths from the
    // outgoing env would otherwise carry over into the new env. Without
    // `applyPinnedColumnSizes`, sidebar inherits the previous task's
    // (possibly user-resized) width.
    const resizeView = vi.fn();
    vi.mocked(getRootSplitview).mockImplementation(
      () =>
        ({
          length: 3,
          getViewSize: () => 800,
          resizeView,
        }) as unknown as NonNullable<ReturnType<typeof getRootSplitview>>,
    );

    performEnvSwitch(makeParams());

    expect(resizeView).toHaveBeenCalledWith(0, 300); // sidebar default (global)
    expect(resizeView).toHaveBeenCalledWith(2, 350); // right default
    // center column is at index 1 and is not pinned — must not be resized.
    expect(resizeView).not.toHaveBeenCalledWith(1, expect.anything());
  });

  it("ignores the saved sidebar size and uses the global width instead", () => {
    // The sidebar is a GLOBAL pref shared across tasks, so the incoming env's
    // saved sidebar size (420 below) must NOT win — it routes through
    // getPinnedWidth (mocked to 300 for the sidebar).
    const savedLayout = {
      grid: {
        root: {
          type: "branch" as const,
          data: [
            { type: "leaf", data: { id: "g1", views: ["sidebar"] }, size: 420 },
            { type: "leaf", data: { id: "g2", views: ["chat"] }, size: 380 },
          ],
        },
        height: 600,
        width: 800,
        orientation: "HORIZONTAL" as const,
      },
      panels: { sidebar: { contentComponent: "sidebar" }, chat: { contentComponent: "chat" } },
      activeGroup: "g1",
    };
    vi.mocked(getEnvLayout).mockReturnValue(
      savedLayout as unknown as ReturnType<typeof getEnvLayout>,
    );

    // Saved exists → fast path uses savedLayoutMatchesLive. Force true.
    vi.mocked(savedLayoutMatchesLive).mockReturnValue(true);

    const resizeView = vi.fn();
    vi.mocked(getRootSplitview).mockImplementation(
      () =>
        ({
          length: 2,
          getViewSize: () => 800,
          resizeView,
        }) as unknown as NonNullable<ReturnType<typeof getRootSplitview>>,
    );

    performEnvSwitch(makeParams());

    // Saved sidebar size (420) is ignored; the global default (300) wins.
    expect(resizeView).toHaveBeenCalledWith(0, 300);
    expect(resizeView).not.toHaveBeenCalledWith(0, 420);
  });

  it("uses the saved size for RIGHT but the global width for the sidebar", () => {
    // Cover the 3-column saved-layout path so the right column's
    // saved-size branch is exercised. The previous test exits the loop
    // before reaching the right column because sv.length = 2.
    const savedLayout = {
      grid: {
        root: {
          type: "branch" as const,
          data: [
            { type: "leaf", data: { id: "g1", views: ["sidebar"] }, size: 420 },
            { type: "leaf", data: { id: "g2", views: ["chat"] }, size: 760 },
            { type: "leaf", data: { id: "g3", views: ["files"] }, size: 420 },
          ],
        },
        height: 600,
        width: 1600,
        orientation: "HORIZONTAL" as const,
      },
      panels: {
        sidebar: { contentComponent: "sidebar" },
        chat: { contentComponent: "chat" },
        files: { contentComponent: "files" },
      },
      activeGroup: "g1",
    };
    vi.mocked(getEnvLayout).mockReturnValue(
      savedLayout as unknown as ReturnType<typeof getEnvLayout>,
    );
    vi.mocked(savedLayoutMatchesLive).mockReturnValue(true);

    const resizeView = vi.fn();
    vi.mocked(getRootSplitview).mockImplementation(
      () =>
        ({
          length: 3,
          getViewSize: () => 800,
          resizeView,
        }) as unknown as NonNullable<ReturnType<typeof getRootSplitview>>,
    );

    performEnvSwitch(makeParams());

    // Sidebar ignores its saved size (420) → global default 300; the right
    // column still restores its saved size (420). Center (index 1) is not
    // pinned and is skipped.
    expect(resizeView).toHaveBeenCalledWith(0, 300);
    expect(resizeView).toHaveBeenCalledWith(2, 420);
    expect(resizeView).not.toHaveBeenCalledWith(1, expect.anything());

    // The saved right width is also forwarded to applyLayoutFixups so the
    // fixups pass anchors the right target to the per-env saved width (420)
    // rather than re-capturing dockview's transient post-fromJSON live size.
    expect(applyLayoutFixups).toHaveBeenCalledWith(expect.anything(), 420);
  });
});
