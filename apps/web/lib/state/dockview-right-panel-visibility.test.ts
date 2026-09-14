import { beforeEach, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { removeEnvMaximizeState, setEnvLayout } from "@/lib/local-storage";

const CENTER_COLUMN_ID = "center";
const CENTER_GROUP_ID = "group-center";
const RIGHT_COLUMN_ID = "right";
const RIGHT_GROUP_ID = "group-right-top";
const SESSION_PANEL_ID = "session:session-a";
const PR_DETAILS_PANEL_ID = "pr-detail";
const PR_DETAILS_PANEL_TITLE = "PR Details";
const MOCK_TERMINAL = vi.hoisted(() => ({ id: "terminal-default" }));

vi.mock("@/lib/local-storage", () => ({
  setEnvLayout: vi.fn(),
  getEnvLayout: vi.fn(() => null),
  getEnvLayoutProfile: vi.fn(() => null),
  setEnvLayoutProfile: vi.fn(),
  getEnvMaximizeState: vi.fn(() => null),
  setEnvMaximizeState: vi.fn(),
  removeEnvMaximizeState: vi.fn(),
  getGlobalSidebarWidth: vi.fn(() => null),
  setGlobalSidebarWidth: vi.fn(),
  clearGlobalSidebarWidth: vi.fn(),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseByEnv: vi.fn(), reconcile: vi.fn() },
}));

vi.mock("./dockview-scroll-preserve", () => ({
  preserveChatScrollDuringLayout: vi.fn(),
}));

vi.mock("./dockview-measure", () => ({
  measureDockviewContainer: vi.fn(() => ({ width: 800, height: 600 })),
}));

vi.mock("./dockview-pinned-enforce", () => ({
  enforcePinnedTargets: vi.fn(),
}));

vi.mock("./dockview-layout-builders", () => ({
  applyLayoutFixups: vi.fn(() => ({
    sidebarGroupId: "g-sidebar",
    centerGroupId: "g-center",
    rightTopGroupId: "g-right-top",
    rightBottomGroupId: "g-right-bottom",
  })),
  focusOrAddPanel: vi.fn(),
}));

vi.mock("./layout-manager", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./layout-manager")>();
  return {
    ...actual,
    SIDEBAR_GROUP: "sidebar",
    CENTER_GROUP: "group-center",
    RIGHT_TOP_GROUP: "group-right-top",
    RIGHT_BOTTOM_GROUP: "group-right-bottom",
    TERMINAL_DEFAULT_ID: MOCK_TERMINAL.id,
    LAYOUT_SIDEBAR_RATIO: 0.2,
    LAYOUT_RIGHT_RATIO: 0.25,
    LAYOUT_PINNED_MIN_PX: 200,
    computeSidebarMaxPx: vi.fn(() => 350),
    computeRightMaxPx: vi.fn(() => 500),
    getPresetLayout: vi.fn(() => ({ columns: [] })),
    applyLayout: vi.fn(() => ({
      sidebarGroupId: "g-sidebar",
      centerGroupId: "g-center",
      rightTopGroupId: "g-right-top",
      rightBottomGroupId: "g-right-bottom",
    })),
    getPinnedWidth: vi.fn(() => 350),
    getRootSplitview: vi.fn(() => null),
    fromDockviewApi: vi.fn(() => ({ columns: [] })),
    filterEphemeral: vi.fn((state: unknown) => state),
    defaultLayout: vi.fn(() => ({ columns: [] })),
    mergeCurrentPanelsIntoPreset: vi.fn((_api: unknown, preset: unknown) => preset),
    toSerializedDockview: vi.fn((state: unknown) => state),
    injectIntentPanels: vi.fn(),
    applyActivePanelOverrides: vi.fn(),
    resolveNamedIntent: vi.fn(),
    setPinnedTarget: vi.fn(),
    clearPinnedTarget: vi.fn(),
    getPinnedTarget: vi.fn(() => undefined),
    layoutStructuresMatch: vi.fn(() => false),
    savedLayoutMatchesLive: vi.fn(() => false),
  };
});

import { applyLayout, defaultLayout, fromDockviewApi } from "./layout-manager";
import { captureRightPane } from "./dockview-right-pane";
import { hasRightColumn, useDockviewStore } from "./dockview-store";

function makeStoreApi(): DockviewApi {
  return {
    width: 800,
    height: 600,
    panels: [],
    groups: [],
    layout: vi.fn(),
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({ columns: [{ id: CENTER_COLUMN_ID }] })),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
    activeGroup: null,
    hasMaximizedGroup: vi.fn(() => false),
    exitMaximizedGroup: vi.fn(),
    onDidActivePanelChange: vi.fn(() => ({ dispose: vi.fn() })),
  } as unknown as DockviewApi;
}

function flushRaf(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

function resetStore() {
  vi.clearAllMocks();
  useDockviewStore.setState({
    api: null,
    currentLayoutEnvId: null,
    preMaximizeLayout: null,
    maximizedGroupId: null,
    isRestoringLayout: false,
    pinnedWidths: new Map(),
    userDefaultLayout: null,
    userDefaultLayoutProfile: { kind: "built-in", id: "default" },
    activeLayoutProfile: { kind: "built-in", id: "default" },
    defaultPreset: "default",
    rightPanelsVisible: true,
    rightPaneVisible: true,
    rightPaneAvailable: false,
    hiddenRightPane: null,
  });
}

beforeEach(resetStore);

it("hides the actual rightmost Plan region instead of rebuilding the default sidebar", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
          },
        ],
      },
      {
        id: "plan",
        groups: [{ id: "group-plan", panels: [{ id: "plan", component: "plan", title: "Plan" }] }],
      },
    ],
  });
  useDockviewStore.setState({ api, rightPanelsVisible: true, defaultPreset: "plan" });

  useDockviewStore.getState().toggleRightPanels();

  const appliedState = vi.mocked(applyLayout).mock.calls.at(-1)?.[1];
  expect(appliedState?.columns.map((column) => column.id)).toEqual([CENTER_COLUMN_ID]);
  await flushRaf();
});

it("retains the hidden pane when applying a toggle layout fails", () => {
  const api = makeStoreApi();
  const fullLayout = {
    rootOrientation: "HORIZONTAL" as const,
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
          },
        ],
      },
      {
        id: "plan",
        groups: [{ id: "group-plan", panels: [{ id: "plan", component: "plan", title: "Plan" }] }],
      },
    ],
  };
  const captured = captureRightPane(fullLayout)!;
  vi.mocked(fromDockviewApi).mockReturnValue(captured.layout);
  vi.mocked(applyLayout).mockImplementationOnce(() => {
    throw new Error("layout failed");
  });
  useDockviewStore.setState({
    api,
    hiddenRightPane: captured.hiddenRightPane,
    rightPaneVisible: false,
    rightPaneAvailable: true,
  });

  useDockviewStore.getState().toggleRightPanels();

  expect(useDockviewStore.getState().hiddenRightPane).toEqual(captured.hiddenRightPane);
  expect(useDockviewStore.getState().rightPaneVisible).toBe(false);
  expect(useDockviewStore.getState().rightPaneAvailable).toBe(true);
  expect(useDockviewStore.getState().isRestoringLayout).toBe(false);
});

it("recognizes the right column from its stable column or group identity", () => {
  expect(
    hasRightColumn({
      columns: [
        { id: CENTER_COLUMN_ID, groups: [{ id: CENTER_GROUP_ID, panels: [] }] },
        { id: RIGHT_COLUMN_ID, groups: [] },
      ],
    }),
  ).toBe(true);
  expect(
    hasRightColumn({
      columns: [{ id: "custom-column", groups: [{ id: RIGHT_GROUP_ID, panels: [] }] }],
    }),
  ).toBe(true);
  expect(
    hasRightColumn({
      columns: [{ id: CENTER_COLUMN_ID, groups: [{ id: CENTER_GROUP_ID, panels: [] }] }],
    }),
  ).toBe(false);
});

it("does not fabricate a right column when the layout has one workbench region", async () => {
  const api = makeStoreApi();
  vi.mocked(defaultLayout).mockReturnValue({
    columns: [
      {
        id: RIGHT_COLUMN_ID,
        pinned: true,
        groups: [
          {
            id: RIGHT_GROUP_ID,
            panels: [
              { id: "files", component: "files", title: "Files" },
              { id: "changes", component: "changes", title: "Changes" },
            ],
          },
          {
            id: "group-right-bottom",
            panels: [{ id: MOCK_TERMINAL.id, component: "terminal", title: "Terminal" }],
          },
        ],
      },
    ],
  });
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              { id: "browser", component: "browser", title: "Browser" },
              {
                id: PR_DETAILS_PANEL_ID,
                component: PR_DETAILS_PANEL_ID,
                title: PR_DETAILS_PANEL_TITLE,
              },
              { id: "files", component: "files", title: "Files" },
              { id: "changes", component: "changes", title: "Changes" },
              { id: MOCK_TERMINAL.id, component: "terminal", title: "Terminal" },
            ],
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({
    api,
    rightPanelsVisible: false,
    defaultPreset: "compact",
    activeLayoutProfile: { kind: "built-in", id: "compact" },
  });

  useDockviewStore.getState().toggleRightPanels();

  expect(applyLayout).not.toHaveBeenCalled();
  expect(defaultLayout).not.toHaveBeenCalled();
  expect(useDockviewStore.getState().rightPaneAvailable).toBe(false);
  await flushRaf();
});

it("does not mutate the temporary maximized layout", () => {
  const api = makeStoreApi();
  useDockviewStore.setState({
    api,
    rightPanelsVisible: true,
    preMaximizeLayout: {
      columns: [
        {
          id: CENTER_COLUMN_ID,
          groups: [
            {
              id: CENTER_GROUP_ID,
              panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
            },
          ],
        },
        {
          id: RIGHT_COLUMN_ID,
          groups: [
            { id: RIGHT_GROUP_ID, panels: [{ id: "files", component: "files", title: "Files" }] },
          ],
        },
      ],
    },
  });

  useDockviewStore.getState().toggleRightPanels();

  expect(applyLayout).not.toHaveBeenCalled();
  expect(api.layout).not.toHaveBeenCalled();
  expect(useDockviewStore.getState().rightPanelsVisible).toBe(true);
});

it("keeps visibility coherent through maximize, exit, and regular-layout persistence", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
          },
        ],
      },
      {
        id: RIGHT_COLUMN_ID,
        groups: [
          { id: RIGHT_GROUP_ID, panels: [{ id: "files", component: "files", title: "Files" }] },
        ],
      },
    ],
  });
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: "env-maximize",
    rightPanelsVisible: false,
  });

  useDockviewStore.getState().maximizeGroup(CENTER_GROUP_ID);
  expect(useDockviewStore.getState().preMaximizeLayout).not.toBeNull();
  expect(useDockviewStore.getState().rightPanelsVisible).toBe(true);
  await flushRaf();

  useDockviewStore.getState().exitMaximizedLayout();
  expect(useDockviewStore.getState().preMaximizeLayout).toBeNull();
  expect(useDockviewStore.getState().rightPanelsVisible).toBe(true);
  await flushRaf();

  expect(removeEnvMaximizeState).toHaveBeenCalledWith("env-maximize");
  expect(setEnvLayout).toHaveBeenCalledWith("env-maximize", expect.anything());
});

it("preserves right-named panels that belong to a mixed center column", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              { id: "files", component: "files", title: "Files" },
              {
                id: PR_DETAILS_PANEL_ID,
                component: PR_DETAILS_PANEL_ID,
                title: PR_DETAILS_PANEL_TITLE,
              },
            ],
          },
        ],
      },
      {
        id: RIGHT_COLUMN_ID,
        groups: [
          {
            id: RIGHT_GROUP_ID,
            panels: [{ id: "terminal", component: "terminal", title: "Terminal" }],
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, rightPanelsVisible: true, defaultPreset: "default" });

  useDockviewStore.getState().toggleRightPanels();

  const appliedState = vi.mocked(applyLayout).mock.calls.at(-1)?.[1];
  expect(appliedState?.columns.map((column) => column.id)).toEqual([CENTER_COLUMN_ID]);
  expect(appliedState?.columns[0]?.groups[0]?.panels.map((panel) => panel.id)).toEqual([
    SESSION_PANEL_ID,
    "files",
    PR_DETAILS_PANEL_ID,
  ]);
  await flushRaf();
});

it("removes an identified right column even when it contains a custom panel", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
          },
        ],
      },
      {
        id: RIGHT_COLUMN_ID,
        groups: [
          { id: RIGHT_GROUP_ID, panels: [{ id: "plugin", component: "plugin", title: "Plugin" }] },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, rightPanelsVisible: true, defaultPreset: "default" });

  useDockviewStore.getState().toggleRightPanels();

  const appliedState = vi.mocked(applyLayout).mock.calls.at(-1)?.[1];
  expect(appliedState?.columns.map((column) => column.id)).toEqual([CENTER_COLUMN_ID]);
  await flushRaf();
});

it("does not restore a fabricated right column when only the center remains", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              { id: "files", component: "files", title: "Files" },
            ],
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, rightPanelsVisible: false, defaultPreset: "default" });

  useDockviewStore.getState().toggleRightPanels();

  expect(applyLayout).not.toHaveBeenCalled();
  expect(defaultLayout).not.toHaveBeenCalled();
  expect(useDockviewStore.getState().rightPaneAvailable).toBe(false);
  await flushRaf();
});

it("keeps a center fallback PR Details tab when hiding right panels", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              {
                id: PR_DETAILS_PANEL_ID,
                component: PR_DETAILS_PANEL_ID,
                title: PR_DETAILS_PANEL_TITLE,
              },
            ],
          },
        ],
      },
      {
        id: RIGHT_COLUMN_ID,
        pinned: true,
        groups: [
          { id: RIGHT_GROUP_ID, panels: [{ id: "files", component: "files", title: "Files" }] },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, rightPanelsVisible: true, defaultPreset: "default" });

  useDockviewStore.getState().toggleRightPanels();

  const appliedState = vi.mocked(applyLayout).mock.calls.at(-1)?.[1];
  expect(appliedState?.columns.map((column) => column.id)).toEqual([CENTER_COLUMN_ID]);
  expect(appliedState?.columns[0]?.groups[0]?.panels.map((panel) => panel.id)).toEqual([
    SESSION_PANEL_ID,
    PR_DETAILS_PANEL_ID,
  ]);
  await flushRaf();
});

it("does not show a default sidebar when showing from a single workbench region", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              {
                id: PR_DETAILS_PANEL_ID,
                component: PR_DETAILS_PANEL_ID,
                title: PR_DETAILS_PANEL_TITLE,
              },
            ],
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, rightPanelsVisible: false, defaultPreset: "default" });

  useDockviewStore.getState().toggleRightPanels();

  expect(applyLayout).not.toHaveBeenCalled();
  expect(defaultLayout).not.toHaveBeenCalled();
  expect(useDockviewStore.getState().rightPaneAvailable).toBe(false);
  await flushRaf();
});

it("persists a settled visibility toggle for the current environment", async () => {
  const api = makeStoreApi();
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: CENTER_COLUMN_ID,
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
          },
        ],
      },
      {
        id: RIGHT_COLUMN_ID,
        groups: [
          { id: RIGHT_GROUP_ID, panels: [{ id: "files", component: "files", title: "Files" }] },
        ],
      },
    ],
  });
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: "env-1",
    rightPanelsVisible: true,
    defaultPreset: "default",
  });

  useDockviewStore.getState().toggleRightPanels();
  await flushRaf();

  expect(setEnvLayout).toHaveBeenCalledWith("env-1", expect.anything());
});
