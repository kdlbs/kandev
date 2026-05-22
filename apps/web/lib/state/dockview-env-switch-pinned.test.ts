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
    getPinnedWidth: vi.fn(() => 350),
  };
});

import { getEnvLayout } from "@/lib/local-storage";
import { getRootSplitview, savedLayoutMatchesLive } from "./layout-manager";

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

    expect(resizeView).toHaveBeenCalledWith(0, 350);
    expect(resizeView).toHaveBeenCalledWith(2, 350);
    // center column is at index 1 and is not pinned — must not be resized.
    expect(resizeView).not.toHaveBeenCalledWith(1, expect.anything());
  });

  it("uses the saved layout's per-column sizes when present", () => {
    // When a saved layout exists for the incoming env, its serialized
    // grid.root.data[i].size wins over the ratio-based default.
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

    expect(resizeView).toHaveBeenCalledWith(0, 420);
    // Only sidebar/right are resized; with 2 sv slots and 3 columns, the
    // loop bound is min(3, 2) = 2, so center (index 1) is the last iterated
    // but since columns[1] = "center" (not pinned), no resize fires. Right
    // is at index 2 in the column list which exceeds sv.length so isn't
    // hit; this scenario covers the saved-sidebar restoration path only.
  });
});
