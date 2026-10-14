import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewApi, SerializedDockview } from "dockview-react";
import type { LayoutState } from "./layout-manager";
import { performLayoutSwitch, useDockviewStore } from "./dockview-store";

vi.mock("@/lib/local-storage", () => ({
  getEnvLayout: vi.fn(() => null),
  getEnvLayoutProfile: vi.fn(() => null),
  setEnvLayout: vi.fn(() => true),
  setEnvLayoutProfile: vi.fn(),
  getEnvMaximizeState: vi.fn(() => null),
  setEnvMaximizeState: vi.fn(),
  removeEnvMaximizeState: vi.fn(),
  getGlobalSidebarWidth: vi.fn(() => null),
  getManualRightWidth: vi.fn(() => null),
  getSessionStorage: vi.fn((_key: string, fallback: string[]) => fallback),
  setGlobalSidebarWidth: vi.fn(),
  clearGlobalSidebarWidth: vi.fn(),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: {
    releaseByEnv: vi.fn(),
    reconcile: vi.fn(),
  },
}));

vi.mock("./layout-manager", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./layout-manager")>();
  return {
    ...actual,
    fromDockviewApi: vi.fn(() => ({ columns: [] })),
  };
});

import {
  getEnvLayout,
  getEnvMaximizeState,
  setEnvLayout,
  setEnvMaximizeState,
} from "@/lib/local-storage";
import { fromDockviewApi } from "./layout-manager";
const CENTER_GROUP_ID = "group-center";

function makeMockApi(): DockviewApi {
  return {
    width: 800,
    height: 600,
    panels: [],
    groups: [],
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({})),
    layout: vi.fn(),
    activeGroup: null,
    onDidActivePanelChange: vi.fn(() => ({ dispose: vi.fn() })),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
    hasMaximizedGroup: vi.fn(() => false),
  } as unknown as DockviewApi;
}

function flushRaf(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  requestAnimationFrame(() => resolve());
  return promise;
}

function maximizeOverlay() {
  return {
    grid: {
      root: {
        type: "branch",
        size: 600,
        data: [
          {
            type: "leaf",
            size: 200,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          { type: "leaf", size: 600, data: { id: "g-max", views: ["chat"], activeView: "chat" } },
        ],
      },
      height: 600,
      width: 800,
      orientation: "HORIZONTAL",
    },
    panels: { chat: { id: "chat", contentComponent: "chat" } },
    activeGroup: "g-max",
  };
}

async function keepsActiveSessionAfterMaximizeExit(): Promise<void> {
  const api = makeMockApi();
  let appliedLayout = {} as SerializedDockview;
  vi.mocked(api.fromJSON).mockImplementation((layout) => {
    appliedLayout = layout;
  });
  vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
  const staleSessionId = "session:removed";
  const activeSessionPanelId = "session:session-b";
  const maximizedLayout = {
    ...maximizeOverlay(),
    grid: {
      ...maximizeOverlay().grid,
      root: {
        type: "leaf",
        size: 600,
        data: {
          id: "g-max",
          views: [activeSessionPanelId],
          activeView: activeSessionPanelId,
        },
      },
    },
    panels: {
      [activeSessionPanelId]: { id: activeSessionPanelId, contentComponent: "chat" },
    },
  };
  vi.mocked(getEnvMaximizeState).mockReturnValue({
    maximizedDockviewJson: maximizedLayout,
    preMaximizeLayout: {
      columns: [
        {
          id: "center",
          groups: [
            {
              id: CENTER_GROUP_ID,
              panels: [{ id: staleSessionId, component: "chat", title: "Agent" }],
              activePanel: staleSessionId,
            },
          ],
        },
      ],
    },
  });
  useDockviewStore.setState({ api, currentLayoutEnvId: "env-a" });

  useDockviewStore.getState().switchEnvLayout("env-a", "env-b", "session-b", ["session-b"]);

  const restoredPreMaximizeLayout = useDockviewStore.getState().preMaximizeLayout;
  expect(restoredPreMaximizeLayout?.columns[0]?.groups[0]).toMatchObject({
    id: CENTER_GROUP_ID,
    panels: [{ id: activeSessionPanelId, params: { sessionId: "session-b" } }],
    activePanel: activeSessionPanelId,
  });
  expect(Object.keys(appliedLayout.panels)).toContain(activeSessionPanelId);
  useDockviewStore.getState().exitMaximizedLayout();
  await flushRaf();

  const persistedLayout = vi
    .mocked(setEnvLayout)
    .mock.calls.find(([envId]) => envId === "env-b")?.[1] as {
    panels?: Record<string, unknown>;
  };
  expect(Object.keys(persistedLayout.panels ?? {})).toContain(activeSessionPanelId);
  expect(Object.keys(persistedLayout.panels ?? {})).not.toContain(staleSessionId);
}

function seedIncompleteAdoption(): {
  activeSessionId: string;
  activePanelId: string;
  siblingPanelId: string;
  foreignSessionId: string;
  foreignPanelId: string;
} {
  const activeSessionId = "session-b";
  const activePanelId = `session:${activeSessionId}`;
  const siblingSessionId = "session-sibling";
  const siblingPanelId = `session:${siblingSessionId}`;
  const foreignSessionId = "session-foreign";
  const foreignPanelId = `session:${foreignSessionId}`;
  const baseOverlay = maximizeOverlay();
  vi.mocked(getEnvMaximizeState).mockReturnValue({
    maximizedDockviewJson: {
      ...baseOverlay,
      grid: {
        ...baseOverlay.grid,
        root: {
          type: "branch",
          size: 600,
          data: [
            {
              type: "leaf",
              size: 200,
              data: { id: "g-sidebar", views: ["files"], activeView: "files" },
            },
            {
              type: "leaf",
              size: 600,
              data: {
                id: "g-max",
                views: [activePanelId, foreignPanelId],
                activeView: activePanelId,
              },
            },
          ],
        },
      },
      panels: {
        [activePanelId]: {
          id: activePanelId,
          contentComponent: "chat",
          params: { sessionId: activeSessionId },
        },
        [foreignPanelId]: {
          id: foreignPanelId,
          contentComponent: "chat",
          params: { sessionId: foreignSessionId },
        },
      },
    },
    preMaximizeLayout: {
      columns: [
        {
          id: "center",
          groups: [
            {
              id: CENTER_GROUP_ID,
              panels: [
                {
                  id: activePanelId,
                  component: "chat",
                  title: "Agent",
                  params: { sessionId: activeSessionId },
                },
                {
                  id: siblingPanelId,
                  component: "chat",
                  title: "Sibling",
                  params: { sessionId: siblingSessionId },
                },
                {
                  id: foreignPanelId,
                  component: "chat",
                  title: "Foreign",
                  params: { sessionId: foreignSessionId },
                },
              ],
              activePanel: activePanelId,
            },
          ],
        },
      ],
    },
  });
  return { activeSessionId, activePanelId, siblingPanelId, foreignSessionId, foreignPanelId };
}

async function preservesSiblingSessionsDuringIncompleteAdoption(): Promise<void> {
  const api = makeMockApi();
  let appliedLayout = {} as SerializedDockview;
  vi.mocked(api.fromJSON).mockImplementation((layout) => {
    appliedLayout = layout;
  });
  vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
  const { activeSessionId, activePanelId, siblingPanelId, foreignSessionId, foreignPanelId } =
    seedIncompleteAdoption();
  useDockviewStore.setState({ api, currentLayoutEnvId: null });

  performLayoutSwitch(null, "env-b", activeSessionId, [activeSessionId], {
    sessionListRestoreState: {
      loaded: false,
      knownForeignSessionIds: new Set([foreignSessionId]),
    },
  });
  expect(appliedLayout.panels).not.toHaveProperty(foreignPanelId);

  const restoredPreMaximizeLayout = useDockviewStore.getState().preMaximizeLayout;
  expect(restoredPreMaximizeLayout?.columns[0]?.groups[0]?.panels).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: activePanelId }),
      expect.objectContaining({ id: siblingPanelId }),
    ]),
  );
  expect(restoredPreMaximizeLayout?.columns[0]?.groups[0]?.panels).not.toContainEqual(
    expect.objectContaining({ id: foreignPanelId }),
  );

  useDockviewStore.getState().exitMaximizedLayout();
  await flushRaf();

  const persistedLayout = vi
    .mocked(setEnvLayout)
    .mock.calls.find(([envId]) => envId === "env-b")?.[1] as {
    panels?: Record<string, unknown>;
  };
  expect(Object.keys(persistedLayout.panels ?? {})).toContain(siblingPanelId);
  expect(Object.keys(persistedLayout.panels ?? {})).not.toContain(foreignPanelId);
}

function persistAuthoritativeSessionCleanup(): void {
  const api = makeMockApi();
  const activeSessionId = "session-b";
  const activePanelId = `session:${activeSessionId}`;
  const siblingSessionId = "session-sibling";
  const siblingPanelId = `session:${siblingSessionId}`;
  const stalePanelId = "session:saved-stale";
  const preMaximizeLayout: LayoutState = {
    columns: [
      {
        id: "center",
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: activePanelId, component: "chat", title: "Agent" },
              { id: siblingPanelId, component: "chat", title: "Sibling" },
              { id: stalePanelId, component: "chat", title: "Stale" },
            ],
            activePanel: activePanelId,
          },
        ],
      },
    ],
  };
  const maximizedDockviewJson = maximizeOverlay() as SerializedDockview;
  vi.mocked(api.toJSON).mockReturnValue(maximizedDockviewJson);
  vi.mocked(getEnvMaximizeState).mockReturnValue({
    preMaximizeLayout,
    maximizedDockviewJson,
  });
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: "env-b",
    preMaximizeLayout,
  });

  useDockviewStore
    .getState()
    .reconcileMaximizeSessionList(activeSessionId, [activeSessionId, siblingSessionId]);

  const savedState = vi.mocked(setEnvMaximizeState).mock.calls[0]?.[1];
  expect(savedState).toBeDefined();
  const savedPanelIds = (
    (savedState?.preMaximizeLayout as unknown as LayoutState).columns[0]?.groups[0]?.panels ?? []
  ).map((panel) => panel.id);
  expect(savedPanelIds).toEqual(expect.arrayContaining([activePanelId, siblingPanelId]));
  expect(savedPanelIds).not.toContain(stalePanelId);
}

function persistSessionReconciliationBeforeMaximizeSave(): void {
  const api = makeMockApi();
  const activeSessionId = "session-a";
  const siblingSessionId = "session-b";
  const activePanelId = `session:${activeSessionId}`;
  const siblingPanelId = `session:${siblingSessionId}`;
  const stalePanelId = "session:saved-stale";
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: "center",
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [
              { id: activePanelId, component: "chat", title: "Agent" },
              { id: siblingPanelId, component: "chat", title: "Sibling" },
              { id: stalePanelId, component: "chat", title: "Stale" },
            ],
            activePanel: activePanelId,
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, currentLayoutEnvId: "env-b" });

  const pendingFrames: FrameRequestCallback[] = [];
  const requestFrame = vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    pendingFrames.push(callback);
    return pendingFrames.length;
  });
  try {
    useDockviewStore.getState().maximizeGroup(CENTER_GROUP_ID);
    useDockviewStore
      .getState()
      .reconcileMaximizeSessionList(activeSessionId, [activeSessionId, siblingSessionId]);
    expect(getEnvMaximizeState).toHaveBeenCalledWith("env-b");

    const persistFrame = pendingFrames.shift();
    expect(persistFrame).toBeDefined();
    persistFrame?.(0);

    const savedState = vi.mocked(setEnvMaximizeState).mock.calls[0]?.[1];
    expect(savedState).toBeDefined();
    const savedPanelIds = (
      (savedState?.preMaximizeLayout as unknown as LayoutState).columns[0]?.groups[0]?.panels ?? []
    ).map((panel) => panel.id);
    expect(savedPanelIds).toEqual(expect.arrayContaining([activePanelId, siblingPanelId]));
    expect(savedPanelIds).not.toContain(stalePanelId);
  } finally {
    requestFrame.mockRestore();
  }
}

function persistFilteredRestoreSnapshot(): void {
  const api = makeMockApi();
  const activeSessionId = "session-a";
  const siblingSessionId = "session-b";
  const activePanelId = `session:${activeSessionId}`;
  const siblingPanelId = `session:${siblingSessionId}`;
  const stalePanelId = "session:saved-stale";
  const makeLayout = (panelIds: string[]): LayoutState => ({
    columns: [
      {
        id: "center",
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: panelIds.map((id) => ({ id, component: "chat", title: id })),
            activePanel: activePanelId,
          },
        ],
      },
    ],
  });
  const preMaximizeLayout = makeLayout([activePanelId, siblingPanelId]);
  const savedPreMaximizeLayout = makeLayout([activePanelId, siblingPanelId, stalePanelId]);
  const maximizedDockviewJson = maximizeOverlay() as SerializedDockview;
  vi.mocked(api.toJSON).mockReturnValue(maximizedDockviewJson);
  vi.mocked(getEnvMaximizeState).mockReturnValue({
    preMaximizeLayout: savedPreMaximizeLayout,
    maximizedDockviewJson,
  });
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: "env-b",
    preMaximizeLayout,
  });

  useDockviewStore
    .getState()
    .reconcileMaximizeSessionList(activeSessionId, [activeSessionId, siblingSessionId]);

  const savedState = vi.mocked(setEnvMaximizeState).mock.calls[0]?.[1];
  expect(savedState).toBeDefined();
  const savedPanelIds = (
    (savedState?.preMaximizeLayout as unknown as LayoutState).columns[0]?.groups[0]?.panels ?? []
  ).map((panel) => panel.id);
  expect(savedPanelIds).toEqual(expect.arrayContaining([activePanelId, siblingPanelId]));
  expect(savedPanelIds).not.toContain(stalePanelId);
}

function preserveRestoringStateAfterEnvironmentSwitch(): void {
  const api = makeMockApi();
  const activePanelId = "session:session-a";
  vi.mocked(fromDockviewApi).mockReturnValue({
    columns: [
      {
        id: "center",
        groups: [
          {
            id: CENTER_GROUP_ID,
            panels: [{ id: activePanelId, component: "chat", title: "Agent" }],
            activePanel: activePanelId,
          },
        ],
      },
    ],
  });
  useDockviewStore.setState({ api, currentLayoutEnvId: "env-a" });

  const pendingFrames: FrameRequestCallback[] = [];
  const requestFrame = vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    pendingFrames.push(callback);
    return pendingFrames.length;
  });
  try {
    useDockviewStore.getState().maximizeGroup(CENTER_GROUP_ID);
    useDockviewStore.setState({
      currentLayoutEnvId: "env-b",
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: true,
    });

    const persistFrame = pendingFrames.shift();
    expect(persistFrame).toBeDefined();
    persistFrame?.(0);

    expect(useDockviewStore.getState().isRestoringLayout).toBe(true);
    expect(setEnvMaximizeState).not.toHaveBeenCalled();
  } finally {
    requestFrame.mockRestore();
  }
}

describe("environment-switch maximize session filtering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(fromDockviewApi).mockReset().mockReturnValue({ columns: [] });
    vi.mocked(getEnvLayout).mockReset().mockReturnValue(null);
    vi.mocked(getEnvMaximizeState).mockReset().mockReturnValue(null);
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });

  it("persists session reconciliation that occurs before maximize state is saved", () => {
    persistSessionReconciliationBeforeMaximizeSave();
  });

  it("persists stale panels removed during authoritative maximize restore", () => {
    persistFilteredRestoreSnapshot();
  });

  it("does not clear a newer environment restore from a deferred maximize frame", () => {
    preserveRestoringStateAfterEnvironmentSwitch();
  });

  it("filters phantom sessions from the pre-maximize snapshot before persisting after exit", async () => {
    const api = makeMockApi();
    let appliedLayout = {} as SerializedDockview;
    vi.mocked(api.fromJSON).mockImplementation((layout) => {
      appliedLayout = layout;
    });
    vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
    const staleSessionId = "session:phantom";
    const incomingSessionId = "session:session-b";
    const nestedPreMaximizeLayout = {
      columns: [
        {
          id: "center",
          groups: [],
          tree: {
            type: "leaf",
            group: {
              id: CENTER_GROUP_ID,
              panels: [
                { id: staleSessionId, component: "chat", title: "Old session" },
                { id: incomingSessionId, component: "chat", title: "Current session" },
                { id: "plan", component: "plan", title: "Plan" },
              ],
              activePanel: staleSessionId,
            },
          },
        },
      ],
    };
    vi.mocked(getEnvMaximizeState).mockReturnValue({
      maximizedDockviewJson: maximizeOverlay(),
      preMaximizeLayout: nestedPreMaximizeLayout,
    });
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-a" });

    useDockviewStore.getState().switchEnvLayout("env-a", "env-b", "session-b", ["session-b"]);

    const restoredPreMaximizeLayout = useDockviewStore.getState().preMaximizeLayout;
    expect(restoredPreMaximizeLayout?.columns[0].tree).toMatchObject({
      type: "leaf",
      group: {
        panels: [{ id: incomingSessionId }, { id: "plan" }],
        activePanel: incomingSessionId,
      },
    });

    useDockviewStore.getState().exitMaximizedLayout();
    await flushRaf();

    const persistedLayout = vi
      .mocked(setEnvLayout)
      .mock.calls.find(([envId]) => envId === "env-b")?.[1] as {
      panels?: Record<string, unknown>;
    };
    expect(persistedLayout).toBeDefined();
    expect(Object.keys(persistedLayout.panels ?? {})).not.toContain(staleSessionId);
    expect(Object.keys(persistedLayout.panels ?? {})).toContain(incomingSessionId);
  });

  it("persists authoritative session cleanup in the maximize restore blob", () => {
    persistAuthoritativeSessionCleanup();
  });

  it("preserves sibling sessions while the first environment adopts before session hydration", async () => {
    await preservesSiblingSessionsDuringIncompleteAdoption();
  });

  it("keeps the active session in a stale-only pre-maximize group on exit", async () => {
    await keepsActiveSessionAfterMaximizeExit();
  });
});
