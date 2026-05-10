import { describe, it, expect, vi, beforeEach } from "vitest";
import { performEnvSwitch, type EnvSwitchParams } from "./dockview-env-switch";

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
