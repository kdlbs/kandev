import { describe, it, expect, vi, beforeEach } from "vitest";
import { performEnvSwitch, type EnvSwitchParams } from "./dockview-env-switch";

const { CANONICAL_CENTER_GROUP_ID, RIGHT_TOP_GROUP_ID, RIGHT_BOTTOM_GROUP_ID } = vi.hoisted(() => ({
  CANONICAL_CENTER_GROUP_ID: "group-center",
  RIGHT_TOP_GROUP_ID: "group-right-top",
  RIGHT_BOTTOM_GROUP_ID: "group-right-bottom",
}));

vi.mock("@/lib/local-storage", () => ({
  getEnvLayout: vi.fn(() => null),
  getManualRightWidth: vi.fn(() => null),
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
  isCenterCandidateGroupId: vi.fn(
    (groupId: string) =>
      groupId !== "group-sidebar" &&
      groupId !== RIGHT_TOP_GROUP_ID &&
      groupId !== RIGHT_BOTTOM_GROUP_ID,
  ),
  setPinnedTarget: vi.fn(),
  CENTER_GROUP: CANONICAL_CENTER_GROUP_ID,
  RIGHT_TOP_GROUP: RIGHT_TOP_GROUP_ID,
  RIGHT_BOTTOM_GROUP: RIGHT_BOTTOM_GROUP_ID,
}));

import { getEnvLayout } from "@/lib/local-storage";
import { layoutStructuresMatch, savedLayoutMatchesLive } from "./layout-manager";

const NEW_SESSION_ID = "new-session";
const OLD_SESSION_PANEL_ID = "session:old-session";
const SIBLING_SESSION_ID = "sibling-session";
const NEW_SESSION_PANEL_ID = `session:${NEW_SESSION_ID}`;
const SIBLING_SESSION_PANEL_ID = `session:${SIBLING_SESSION_ID}`;
const CENTER_GROUP_ID = "center-group";

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
    activeSessionId: NEW_SESSION_ID,
    safeWidth: 800,
    safeHeight: 600,
    buildDefault: vi.fn(),
    getDefaultLayout: vi.fn(() => ({ columns: [] })),
    ...overrides,
  };
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
      { id: OLD_SESSION_PANEL_ID, api: { component: "chat" } },
      { id: "changes", api: { component: "changes" } },
      { id: "terminal-default", api: { component: "terminal" } },
    ];
    const groupId = CENTER_GROUP_ID;
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

describe("performEnvSwitch layout-owned review panel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps layout-owned PR Details during a fresh fast switch", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const close = vi.fn();
    const prDetails = {
      id: "pr-detail",
      api: { component: "pr-detail", close },
      group: { id: RIGHT_TOP_GROUP_ID, panels: [] },
    };
    const params = makeParams({
      api: {
        ...makeMockApi(),
        panels: [prDetails],
        groups: [prDetails.group],
      } as unknown as EnvSwitchParams["api"],
    });

    performEnvSwitch(params);

    expect(close).not.toHaveBeenCalled();
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
    const groupId = CENTER_GROUP_ID;
    const outgoingPanels = [
      {
        id: OLD_SESSION_PANEL_ID,
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

  it("restores sibling session tabs when the fast path replaces the outgoing session", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);

    type TestPanel = {
      id: string;
      api: { component: string; isActive?: boolean; close: () => void };
      group: { id: string; panels: TestPanel[] };
    };

    const group = { id: CENTER_GROUP_ID, panels: [] as TestPanel[] };
    const panels: TestPanel[] = [];
    const removePanel = (id: string) => {
      const panelIndex = panels.findIndex((p) => p.id === id);
      if (panelIndex >= 0) panels.splice(panelIndex, 1);
      const groupIndex = group.panels.findIndex((p) => p.id === id);
      if (groupIndex >= 0) group.panels.splice(groupIndex, 1);
    };
    const closeStale = vi.fn(() => removePanel(OLD_SESSION_PANEL_ID));
    const stalePanel: TestPanel = {
      id: OLD_SESSION_PANEL_ID,
      api: { component: "chat", isActive: true, close: closeStale },
      group,
    };
    panels.push(stalePanel);
    group.panels.push(stalePanel);

    const addPanel = vi.fn((opts: { id: string; component: string }) => {
      const panel: TestPanel = {
        id: opts.id,
        api: { component: opts.component, close: () => removePanel(opts.id) },
        group,
      };
      panels.push(panel);
      group.panels.push(panel);
    });
    const api = {
      ...makeMockApi(),
      panels,
      groups: [group],
      getPanel: vi.fn((id: string) => panels.find((p) => p.id === id) ?? null),
      addPanel,
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(
      makeParams({
        api,
        currentSessionIds: [NEW_SESSION_ID, SIBLING_SESSION_ID],
      }),
    );

    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        position: { referenceGroup: group.id, index: 0 },
      }),
    );
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: SIBLING_SESSION_PANEL_ID,
        params: { sessionId: SIBLING_SESSION_ID },
        position: { referenceGroup: group.id, index: 1 },
        inactive: true,
      }),
    );
    expect(closeStale).toHaveBeenCalledOnce();
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

describe("performEnvSwitch missing Agent restoration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("restores and activates Agent in an empty center group", () => {
    const savedLayout = makeHealthyLayoutWith({});
    vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const setActive = vi.fn();
    const centerGroup = { id: CANONICAL_CENTER_GROUP_ID, panels: [] };
    const addPanel = vi.fn(() => ({
      id: NEW_SESSION_PANEL_ID,
      api: { component: "chat", setActive },
      group: centerGroup,
    }));
    const api = {
      ...makeMockApi(),
      panels: [],
      groups: [centerGroup],
      getPanel: vi.fn(() => null),
      addPanel,
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(makeParams({ api }));

    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        position: expect.objectContaining({ referenceGroup: centerGroup.id }),
        inactive: false,
      }),
    );
    expect(setActive).toHaveBeenCalledOnce();
  });

  it("restores Agent beside a saved active Plan tab without stealing focus", () => {
    const savedLayout = makeHealthyLayoutWith({ plan: { contentComponent: "plan" } });
    vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const setActive = vi.fn();
    const planPanel = { id: "plan", api: { component: "plan", isActive: true } };
    const centerGroup = { id: CANONICAL_CENTER_GROUP_ID, panels: [] };
    const planGroup = { id: "center-secondary", panels: [planPanel] };
    const addPanel = vi.fn(() => ({
      id: NEW_SESSION_PANEL_ID,
      api: { component: "chat", setActive },
      group: centerGroup,
    }));
    const api = {
      ...makeMockApi(),
      activePanel: planPanel,
      panels: [planPanel],
      groups: [centerGroup, planGroup],
      getPanel: vi.fn((id: string) => (id === "plan" ? planPanel : null)),
      addPanel,
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(makeParams({ api }));

    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        position: expect.objectContaining({ referenceGroup: centerGroup.id }),
        inactive: true,
      }),
    );
    expect(setActive).not.toHaveBeenCalled();
  });

  it("does not use a configured PR Details group as the Agent restoration anchor", () => {
    const savedLayout = makeHealthyLayoutWith({ "pr-detail": { contentComponent: "pr-detail" } });
    vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);

    const fallbackGroup = { id: "fallback-center", panels: [] };
    const reviewGroup = { id: "custom-pr-details-group", panels: [] };
    const reviewPanel = { id: "pr-detail", api: { component: "pr-detail" }, group: reviewGroup };
    const addPanel = vi.fn(() => ({
      id: NEW_SESSION_PANEL_ID,
      api: { component: "chat", setActive: vi.fn() },
      group: fallbackGroup,
    }));
    const api = {
      ...makeMockApi(),
      panels: [reviewPanel],
      groups: [fallbackGroup, reviewGroup],
      getPanel: vi.fn((id: string) => (id === "pr-detail" ? reviewPanel : null)),
      addPanel,
    } as unknown as EnvSwitchParams["api"];

    performEnvSwitch(makeParams({ api }));

    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: NEW_SESSION_PANEL_ID,
        position: expect.objectContaining({ referenceGroup: fallbackGroup.id }),
      }),
    );
  });
});
