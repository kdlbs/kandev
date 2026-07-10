import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  performEnvSwitch,
  savedRightColumnWidth,
  type EnvSwitchParams,
} from "./dockview-env-switch";
import type { SerializedDockview } from "dockview-react";

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

vi.mock("./layout-manager", () => ({
  fromDockviewApi: vi.fn(() => ({ columns: [] })),
  savedLayoutMatchesLive: vi.fn(() => false),
  layoutStructuresMatch: vi.fn(() => false),
  getRootSplitview: vi.fn(() => null),
  getPinnedWidth: vi.fn(() => 350),
  setPinnedTarget: vi.fn(),
  RIGHT_TOP_GROUP: "group-right-top",
  RIGHT_BOTTOM_GROUP: "group-right-bottom",
}));

import { getEnvLayout } from "@/lib/local-storage";
import { layoutStructuresMatch, savedLayoutMatchesLive } from "./layout-manager";

const NEW_SESSION_PANEL_ID = "session:new-session";

function makeMockApi() {
  return {
    panels: [],
    groups: [],
    layout: vi.fn(),
    fromJSON: vi.fn(),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
  } as unknown as EnvSwitchParams["api"];
}

function makeHealthyLayoutWith(extraPanels: Record<string, { contentComponent: string }>) {
  return {
    grid: {
      root: {
        type: "leaf" as const,
        size: 800,
        data: { id: "g1", views: ["chat"], activeView: "chat" },
      },
      height: 600,
      width: 800,
      orientation: "HORIZONTAL" as const,
    },
    panels: {
      chat: { contentComponent: "chat" },
      ...extraPanels,
    },
    activeGroup: "g1",
  } as unknown as ReturnType<typeof getEnvLayout>;
}

function makeParams(overrides?: Partial<EnvSwitchParams>): EnvSwitchParams {
  return {
    api: makeMockApi(),
    oldEnvId: "old-env",
    newEnvId: "new-env",
    activeSessionId: "new-session",
    safeWidth: 800,
    safeHeight: 600,
    buildDefault: vi.fn(),
    getDefaultLayout: vi.fn(() => ({ columns: [] })),
    ...overrides,
  };
}

function makeTwoLeafSavedLayout(
  leaves: Array<{ id: string; views: string[]; activeView: string }>,
  activeGroup: string,
): ReturnType<typeof getEnvLayout> {
  return {
    grid: {
      root: {
        type: "branch" as const,
        data: leaves.map((leaf) => ({ type: "leaf", data: leaf })),
      },
      height: 600,
      width: 800,
      orientation: "HORIZONTAL" as const,
    },
    panels: { chat: { contentComponent: "chat" } },
    activeGroup,
  } as unknown as ReturnType<typeof getEnvLayout>;
}

describe("performEnvSwitch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.layout on the fast path when structures match", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const params = makeParams();

    performEnvSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
  });

  it("calls api.layout on the fast path when saved layout matches", () => {
    vi.mocked(getEnvLayout).mockReturnValueOnce(makeHealthyLayoutWith({}));
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);
    const params = makeParams();

    performEnvSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
    expect(params.api.fromJSON).not.toHaveBeenCalled();
  });

  it("creates session panel inline on the fast path when it does not exist", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const params = makeParams();

    performEnvSwitch(params);

    expect(params.api.addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        component: "chat",
        params: { sessionId: "new-session" },
      }),
    );
  });

  it("skips addPanel on the fast path when the session panel already exists", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const panel = { id: NEW_SESSION_PANEL_ID, api: { component: "chat" }, group: { id: "g1" } };
    const params = makeParams({
      api: {
        ...makeMockApi(),
        getPanel: vi.fn((id: string) => (id === NEW_SESSION_PANEL_ID ? panel : null)),
      } as unknown as EnvSwitchParams["api"],
    });

    performEnvSwitch(params);

    expect(params.api.addPanel).not.toHaveBeenCalled();
  });

  it.each(["file-editor", "browser", "vscode", "commit-detail", "diff-viewer", "pr-detail"])(
    "skips fast path when saved layout has ephemeral panels (%s)",
    (contentComponent) => {
      const savedLayout = makeHealthyLayoutWith({
        [`preview:${contentComponent}`]: { contentComponent },
      });
      vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
      vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);
      const params = makeParams();

      performEnvSwitch(params);

      expect(params.api.fromJSON).toHaveBeenCalled();
    },
  );

  it("calls api.layout on the slow path (buildDefault fallback)", () => {
    const params = makeParams();

    performEnvSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
    expect(params.buildDefault).toHaveBeenCalledWith(params.api);
  });

  it("preserves the outgoing session panel's tab index when adding the new session on the fast path", () => {
    // Regression: the fast-path used to call addPanel with only
    // { referenceGroup }, so dockview appended the new session tab to the end
    // of the group instead of restoring it to its original slot.
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const groupPanels = [
      { id: "files", api: { component: "files" } },
      { id: "session:old-session", api: { component: "chat" } },
      { id: "changes", api: { component: "changes" } },
      { id: "terminal-default", api: { component: "terminal" } },
    ];
    const groupId = "center-group";
    const outgoing = {
      ...groupPanels[1],
      group: { id: groupId, panels: groupPanels },
    };
    const api = {
      ...makeMockApi(),
      panels: [outgoing],
      groups: [{ id: groupId }],
      getPanel: vi.fn(() => null),
    } as unknown as EnvSwitchParams["api"];
    const params = makeParams({ api });

    performEnvSwitch(params);

    expect(api.addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        position: { referenceGroup: groupId, index: 1 },
      }),
    );
  });
});

describe("performEnvSwitch fast-path group survival", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("adds the incoming session panel before closing the outgoing chat (keeps its group alive)", () => {
    // Regression: when the outgoing session chat is the only surviving panel in
    // its group (e.g. the default layout's chat column), removing it FIRST
    // empties — and so destroys — the group. The captured group id then no
    // longer exists, and post-#1165 the old `referenceGroup: "sidebar"` fallback
    // is dead, so `addPanel` runs with an undefined position and dockview drops
    // the incoming chat into whatever group is active (the terminal),
    // collapsing the grid root to a vertical stack. Adding the incoming panel
    // BEFORE removing the outgoing one keeps the group alive throughout.
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const order: string[] = [];
    const closeOutgoing = vi.fn(() => order.push("close"));
    const groupId = "center-group";
    const outgoingPanels = [
      {
        id: "session:old-session",
        api: { component: "chat", isActive: true, close: closeOutgoing },
      },
    ];
    const outgoing = { ...outgoingPanels[0], group: { id: groupId, panels: outgoingPanels } };
    const addPanel = vi.fn(() => order.push("add"));
    const api = {
      ...makeMockApi(),
      panels: [outgoing],
      groups: [{ id: groupId }],
      getPanel: vi.fn(() => null),
      addPanel,
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(makeParams({ api }));

    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        position: { referenceGroup: groupId, index: 0 },
      }),
    );
    expect(order).toEqual(["add", "close"]);
  });
});

describe("performEnvSwitch slow-path stale session strip", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("closes stale session chat panels after the slow-path fromJSON", () => {
    // Regression: a saved env layout could carry a `session:*` panel from a
    // previously-deleted task (phantom). On the slow-path restore, that
    // panel would land in the live api as a stray tab. replaceStaleSessionPanels
    // must close any session:* panel whose id != the incoming active session.
    vi.mocked(getEnvLayout)
      .mockReturnValueOnce(makeHealthyLayoutWith({}))
      .mockReturnValueOnce(makeHealthyLayoutWith({}));
    // Force the slow path: layouts don't structurally match.
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const closeStale = vi.fn();
    const closeFileEditor = vi.fn();
    const closeKeep = vi.fn();
    const api = {
      ...makeMockApi(),
      // api.fromJSON is a no-op mock; populate `panels` with what would exist
      // post-restore so replaceStaleSessionPanels' filter has something to act on.
      panels: [
        { id: "session:old-session", api: { component: "chat", close: closeStale } },
        { id: NEW_SESSION_PANEL_ID, api: { component: "chat", close: closeKeep } },
        // file editors are NOT session panels — they must NOT be closed.
        { id: "preview:file-editor", api: { component: "file-editor", close: closeFileEditor } },
      ],
      getPanel: vi.fn((id: string) =>
        id === NEW_SESSION_PANEL_ID ? { id: NEW_SESSION_PANEL_ID } : null,
      ),
    } as unknown as EnvSwitchParams["api"];
    const params = makeParams({ api });

    performEnvSwitch(params);

    expect(closeStale).toHaveBeenCalledOnce();
    expect(closeKeep).not.toHaveBeenCalled();
    expect(closeFileEditor).not.toHaveBeenCalled();
    expect(params.api.fromJSON).toHaveBeenCalledOnce();
    // Keep panel already existed, so no addPanel.
    expect(params.api.addPanel).not.toHaveBeenCalled();
  });

  it("anchors the new session to the stale session's group and tab index", () => {
    // Regression: when the saved layout had a phantom session co-tabbed with
    // pr-detail (or other siblings the user dragged into the chat group),
    // simply closing the phantom orphaned the siblings. The new active session
    // would then land as a fresh split next to the sidebar — pulling pr-detail
    // out of the user's grouping. The replacement must land in the phantom's
    // exact (group, index) so siblings stay tabbed with the agent.
    vi.mocked(getEnvLayout)
      .mockReturnValueOnce(makeHealthyLayoutWith({}))
      .mockReturnValueOnce(makeHealthyLayoutWith({}));
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const closeStale = vi.fn();
    const stalePanelId = "session:phantom-from-other-env";
    const groupId = "saved-center-group";
    const groupPanels = [
      { id: stalePanelId, api: { component: "chat", close: closeStale } },
      { id: "pr-detail", api: { component: "pr-detail", close: vi.fn() } },
    ];
    const stale = {
      ...groupPanels[0],
      group: { id: groupId, panels: groupPanels },
    };
    const api = {
      ...makeMockApi(),
      panels: [stale, { id: "pr-detail", api: { component: "pr-detail" }, group: { id: groupId } }],
      groups: [{ id: groupId }],
      // The active session panel does NOT exist yet — that's the whole point;
      // the fromJSON restore only brought back the phantom.
      getPanel: vi.fn(() => null),
    } as unknown as EnvSwitchParams["api"];
    const params = makeParams({ api });

    performEnvSwitch(params);

    expect(api.addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        component: "chat",
        position: { referenceGroup: groupId, index: 0 },
      }),
    );
    expect(closeStale).toHaveBeenCalledOnce();
  });
});

describe("performEnvSwitch slow-path session tab restoration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("restores sibling session tabs when slow-path replacement receives current session ids", () => {
    vi.mocked(getEnvLayout)
      .mockReturnValueOnce(makeHealthyLayoutWith({}))
      .mockReturnValueOnce(makeHealthyLayoutWith({}));
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const closeStale = vi.fn();
    const groupId = "saved-center-group";
    const groupPanels = [
      { id: "session:phantom-from-other-env", api: { component: "chat", close: closeStale } },
    ];
    const stale = {
      ...groupPanels[0],
      group: { id: groupId, panels: groupPanels },
    };
    const panelsById = new Map<
      string,
      { id: string; api: { component: string }; group: unknown }
    >();
    const addPanel = vi.fn((opts: { id: string; component: string }) => {
      panelsById.set(opts.id, {
        id: opts.id,
        api: { component: opts.component },
        group: { id: groupId, panels: [] },
      });
    });
    const api = {
      ...makeMockApi(),
      panels: [stale],
      groups: [{ id: groupId }],
      getPanel: vi.fn((id: string) => panelsById.get(id) ?? null),
      addPanel,
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(
      makeParams({
        api,
        currentSessionIds: ["new-session", "sibling-session"],
      }),
    );

    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        component: "chat",
        position: { referenceGroup: groupId, index: 0 },
      }),
    );
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "session:sibling-session",
        component: "chat",
        params: { sessionId: "sibling-session" },
        position: { referenceGroup: groupId, index: 0 },
        inactive: true,
      }),
    );
    expect(closeStale).toHaveBeenCalledOnce();
  });

  it("skips addPanel when there is no active session (sessionless task)", () => {
    vi.mocked(getEnvLayout)
      .mockReturnValueOnce(makeHealthyLayoutWith({}))
      .mockReturnValueOnce(makeHealthyLayoutWith({}));
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const closeStale = vi.fn();
    const groupId = "g1";
    const groupPanels = [{ id: "session:phantom", api: { component: "chat", close: closeStale } }];
    const stale = { ...groupPanels[0], group: { id: groupId, panels: groupPanels } };
    const api = {
      ...makeMockApi(),
      panels: [stale],
      groups: [{ id: groupId }],
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(makeParams({ api, activeSessionId: null }));

    expect(closeStale).toHaveBeenCalledOnce();
    expect(api.addPanel).not.toHaveBeenCalled();
  });
});

describe("performEnvSwitch fast-path active view restoration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("restores saved per-group active tabs on the fast path", () => {
    // Regression: the fast path skips fromJSON, so per-group active tabs
    // from the outgoing env would persist into the incoming env. The saved
    // layout's activeView for each group must be reapplied.
    const setActiveRight = vi.fn();
    const setActiveCenter = vi.fn();
    const rightGroup = {
      id: "right",
      panels: [
        { id: "plan", api: { setActive: setActiveRight } },
        { id: "files", api: { setActive: vi.fn() } },
      ],
    };
    const centerGroup = {
      id: "center",
      panels: [{ id: NEW_SESSION_PANEL_ID, api: { setActive: setActiveCenter } }],
    };
    const savedLayout = makeTwoLeafSavedLayout(
      [
        { id: "center", views: ["chat"], activeView: "chat" },
        { id: "right", views: ["plan", "files"], activeView: "plan" },
      ],
      "right",
    );
    vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);
    const api = {
      ...makeMockApi(),
      groups: [centerGroup, rightGroup],
      getPanel: vi.fn((id: string) => (id === NEW_SESSION_PANEL_ID ? centerGroup.panels[0] : null)),
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(makeParams({ api }));

    expect(setActiveRight).toHaveBeenCalled();
    // The saved activeGroup ("right") is applied last, so its setActive must
    // be the most recent — otherwise center would steal global focus.
    const lastRightCall = setActiveRight.mock.invocationCallOrder.at(-1) ?? 0;
    const lastCenterCall = setActiveCenter.mock.invocationCallOrder.at(-1) ?? 0;
    expect(lastRightCall).toBeGreaterThan(lastCenterCall);
  });
});

describe("savedRightColumnWidth", () => {
  function makeSaved(children: Array<{ id: string; size: number }>): SerializedDockview {
    return {
      grid: {
        root: {
          type: "branch",
          data: children.map((c) => ({
            type: "leaf",
            data: { id: c.id, views: [] },
            size: c.size,
          })),
        },
        height: 600,
        width: 1600,
        orientation: "HORIZONTAL",
      },
      panels: {},
      activeGroup: undefined,
    } as unknown as SerializedDockview;
  }

  it("returns the saved right size for a 3-column layout (sidebar+center+right)", () => {
    const saved = makeSaved([
      { id: "group-sidebar", size: 300 },
      { id: "group-center", size: 1000 },
      { id: "group-right-top", size: 300 },
    ]);
    expect(savedRightColumnWidth(saved)).toBe(300);
  });

  it("returns the saved right size for a 2-column layout with sidebar hidden", () => {
    // Regression: pre-fix this returned undefined (column-count gate), which
    // caused the right column to fall back to ~450 default on env switch
    // instead of restoring the user's narrow width.
    const saved = makeSaved([
      { id: "group-center", size: 1380 },
      { id: "group-right-top", size: 220 },
    ]);
    expect(savedRightColumnWidth(saved)).toBe(220);
  });

  it("returns undefined for a 2-column layout where the last child is not a right column", () => {
    // 2-column layouts can also be sidebar+center (right hidden); we must NOT
    // mistake the center for the right column.
    const saved = makeSaved([
      { id: "group-sidebar", size: 300 },
      { id: "group-center", size: 1300 },
    ]);
    expect(savedRightColumnWidth(saved)).toBeUndefined();
  });

  it("returns undefined for null input", () => {
    expect(savedRightColumnWidth(null)).toBeUndefined();
  });
});
