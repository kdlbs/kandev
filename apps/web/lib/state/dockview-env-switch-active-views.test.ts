import { beforeEach, expect, it, vi } from "vitest";
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
const NEW_SESSION_PANEL_ID = `session:${NEW_SESSION_ID}`;

type TestPanel = {
  id: string;
  api: { component?: string; setActive?: ReturnType<typeof vi.fn>; close?: () => void };
  group?: TestGroup;
};

type TestGroup = { id: string; panels: TestPanel[] };

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

function makeAgentReviewApi(
  setActiveAgent: ReturnType<typeof vi.fn>,
  setActiveReview: ReturnType<typeof vi.fn>,
  groupId = "center",
) {
  const group: TestGroup = { id: groupId, panels: [] };
  group.panels = [
    { id: NEW_SESSION_PANEL_ID, api: { component: "chat", setActive: setActiveAgent }, group },
    { id: "pr-detail", api: { component: "pr-detail", setActive: setActiveReview }, group },
  ];
  return {
    group,
    api: {
      ...makeMockApi(),
      panels: group.panels,
      groups: [group],
      getPanel: vi.fn((id: string) => group.panels.find((panel) => panel.id === id) ?? null),
    } as unknown as EnvSwitchParams["api"],
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

it("rebinds a saved Chat selection to the incoming Agent panel", () => {
  const setActiveAgent = vi.fn();
  const setActiveReview = vi.fn();
  const savedLayout = makeTwoLeafSavedLayout(
    [{ id: "center", views: ["chat", "pr-detail"], activeView: "chat" }],
    "center",
  );
  const { api } = makeAgentReviewApi(setActiveAgent, setActiveReview);
  vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout);
  vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);

  performEnvSwitch(makeParams({ api }));

  expect(setActiveAgent).toHaveBeenCalledOnce();
  expect(setActiveReview).not.toHaveBeenCalled();
});

it("rebinds a stale saved session selection to the incoming Agent panel", () => {
  const setActiveAgent = vi.fn();
  const setActiveReview = vi.fn();
  const savedLayout = makeTwoLeafSavedLayout(
    [
      {
        id: "center",
        views: [OLD_SESSION_PANEL_ID, "pr-detail"],
        activeView: OLD_SESSION_PANEL_ID,
      },
    ],
    "center",
  );
  const { api } = makeAgentReviewApi(setActiveAgent, setActiveReview);
  vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout);
  vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);

  performEnvSwitch(makeParams({ api }));

  expect(setActiveAgent).toHaveBeenCalledOnce();
  expect(setActiveReview).not.toHaveBeenCalled();
});

it("uses the effective default Agent selection when the target has no saved layout", () => {
  const setActiveAgent = vi.fn();
  const setActiveReview = vi.fn();
  const { api } = makeAgentReviewApi(setActiveAgent, setActiveReview, CANONICAL_CENTER_GROUP_ID);
  vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);

  performEnvSwitch(
    makeParams({
      api,
      getDefaultLayout: vi.fn(() => ({
        columns: [
          {
            id: "center",
            groups: [
              {
                id: CANONICAL_CENTER_GROUP_ID,
                activePanel: "chat",
                panels: [
                  { id: "chat", component: "chat", title: "Agent" },
                  { id: "pr-detail", component: "pr-detail", title: "PR Details" },
                ],
              },
            ],
          },
        ],
      })),
    }),
  );

  expect(setActiveAgent).toHaveBeenCalledOnce();
  expect(setActiveReview).not.toHaveBeenCalled();
});

it("rebinds a stale saved session selection after slow-path replacement", () => {
  const setActiveAgent = vi.fn();
  const setActiveReview = vi.fn();
  const group: TestGroup = { id: "saved-center", panels: [] };
  const panels: TestPanel[] = [];
  const removePanel = (id: string) => {
    const index = panels.findIndex((panel) => panel.id === id);
    if (index >= 0) panels.splice(index, 1);
    const groupIndex = group.panels.findIndex((panel) => panel.id === id);
    if (groupIndex >= 0) group.panels.splice(groupIndex, 1);
  };
  const stale: TestPanel = {
    id: OLD_SESSION_PANEL_ID,
    api: { component: "chat", close: () => removePanel(OLD_SESSION_PANEL_ID) },
    group,
  };
  const review: TestPanel = {
    id: "pr-detail",
    api: { component: "pr-detail", setActive: setActiveReview },
    group,
  };
  panels.push(stale, review);
  group.panels.push(stale, review);
  const addPanel = vi.fn((options: { id: string; component: string }) => {
    const incoming: TestPanel = {
      id: options.id,
      api: { component: options.component, setActive: setActiveAgent },
      group,
    };
    panels.unshift(incoming);
    group.panels.unshift(incoming);
    return incoming;
  });
  const savedLayout = makeTwoLeafSavedLayout(
    [
      {
        id: group.id,
        views: [OLD_SESSION_PANEL_ID, "pr-detail"],
        activeView: OLD_SESSION_PANEL_ID,
      },
    ],
    group.id,
  );
  vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
  vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(false);
  const api = {
    ...makeMockApi(),
    panels,
    groups: [group],
    addPanel,
    getPanel: vi.fn((id: string) => panels.find((panel) => panel.id === id) ?? null),
  } as unknown as EnvSwitchParams["api"];

  performEnvSwitch(makeParams({ api }));

  expect(api.fromJSON).toHaveBeenCalledOnce();
  expect(setActiveAgent).toHaveBeenCalledOnce();
  expect(setActiveReview).not.toHaveBeenCalled();
});

it("keeps a deliberately selected PR Details panel selected", () => {
  const setActiveAgent = vi.fn();
  const setActiveReview = vi.fn();
  const savedLayout = makeTwoLeafSavedLayout(
    [{ id: "center", views: ["chat", "pr-detail"], activeView: "pr-detail" }],
    "center",
  );
  const { api } = makeAgentReviewApi(setActiveAgent, setActiveReview);
  vi.mocked(getEnvLayout).mockReturnValueOnce(savedLayout);
  vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);

  performEnvSwitch(makeParams({ api }));

  expect(setActiveReview).toHaveBeenCalledOnce();
  expect(setActiveAgent).not.toHaveBeenCalled();
});

it("restores saved per-group active tabs on the fast path", () => {
  const setActiveRight = vi.fn();
  const setActiveCenter = vi.fn();
  const rightGroup: TestGroup = {
    id: "right",
    panels: [
      { id: "plan", api: { setActive: setActiveRight } },
      { id: "files", api: { setActive: vi.fn() } },
    ],
  };
  const centerGroup: TestGroup = {
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

  expect(setActiveCenter).toHaveBeenCalledOnce();
  expect(setActiveRight).toHaveBeenCalled();
  const lastRightCall = setActiveRight.mock.invocationCallOrder.at(-1) ?? 0;
  const lastCenterCall = setActiveCenter.mock.invocationCallOrder.at(-1) ?? 0;
  expect(lastRightCall).toBeGreaterThan(lastCenterCall);
});
