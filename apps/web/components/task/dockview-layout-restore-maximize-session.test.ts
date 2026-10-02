import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewReadyEvent, SerializedDockview } from "dockview-react";
import { restoreEnvLayout } from "./dockview-layout-restore";
import { useDockviewStore } from "@/lib/state/dockview-store";
import * as localStorage from "@/lib/local-storage";

const VALID_COMPONENTS = new Set<string>(["chat", "files", "shell", "git", "terminal"]);

function makeSavedLayout() {
  return {
    grid: {
      root: {
        type: "branch" as const,
        size: 600,
        data: [
          {
            type: "leaf" as const,
            size: 350,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          {
            type: "leaf" as const,
            size: 800,
            data: { id: "g-center", views: ["chat"], activeView: "chat" },
          },
          {
            type: "leaf" as const,
            size: 450,
            data: { id: "g-right", views: ["git"], activeView: "git" },
          },
        ],
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL" as const,
    },
    panels: {
      files: { id: "files", contentComponent: "files" },
      chat: { id: "chat", contentComponent: "chat" },
      git: { id: "git", contentComponent: "git" },
    },
    activeGroup: "g-center",
  };
}

function makeMaximizeOverlay(panelId: string) {
  return {
    grid: {
      root: {
        type: "branch" as const,
        size: 600,
        data: [
          {
            type: "leaf" as const,
            size: 300,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          {
            type: "leaf" as const,
            size: 1300,
            data: { id: "g-max", views: [panelId], activeView: panelId },
          },
        ],
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL" as const,
    },
    panels: { [panelId]: { id: panelId, contentComponent: "chat" } },
    activeGroup: "g-max",
  };
}

function makeRestoreApi() {
  return {
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({})),
    layout: vi.fn(),
    hasMaximizedGroup: vi.fn(() => false),
    width: 1600,
    height: 600,
    groups: [],
    panels: [],
    activeGroup: { id: "g-center" },
    getPanel: vi.fn(() => null),
    onDidActiveGroupChange: vi.fn(() => ({ dispose: vi.fn() })),
    onDidLayoutChange: vi.fn(() => ({ dispose: vi.fn() })),
  } as unknown as DockviewReadyEvent["api"];
}

function flushRaf(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  requestAnimationFrame(() => resolve());
  return promise;
}

describe("on-ready maximize restore", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });

  it("preserves the active session and panel params from a stale-only pre-maximize tree through exit", async () => {
    const activeSessionId = "alive";
    const activePanelId = `session:${activeSessionId}`;
    const staleSessionId = "phantom";
    vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(makeSavedLayout());
    vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue({
      maximizedDockviewJson: makeMaximizeOverlay(activePanelId),
      preMaximizeLayout: {
        columns: [
          {
            id: "center",
            groups: [],
            tree: {
              type: "leaf",
              size: 600,
              group: {
                id: "g-center",
                panels: [
                  {
                    id: `session:${staleSessionId}`,
                    component: "chat",
                    title: "Stale session",
                    params: { keep: "value" },
                  },
                ],
                activePanel: `session:${staleSessionId}`,
              },
            },
          },
        ],
      },
    });
    let appliedLayout = {} as SerializedDockview;
    const api = makeRestoreApi();
    vi.mocked(api.fromJSON).mockImplementation((layout) => {
      appliedLayout = layout;
    });
    vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
    const persistLayout = vi.spyOn(localStorage, "setEnvLayout").mockReturnValue(true);
    const appStore = {
      getState: () => ({
        environmentIdBySessionId: {
          [activeSessionId]: "env-live",
          [staleSessionId]: "env-other",
        },
        tasks: { activeTaskId: "task-live", activeSessionId },
        taskSessionsByTask: {
          itemsByTaskId: { "task-live": [{ id: activeSessionId }] },
          loadedByTaskId: { "task-live": true },
        },
      }),
    };
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-live" });

    expect(
      restoreEnvLayout(
        api,
        "env-live",
        appStore as unknown as Parameters<typeof restoreEnvLayout>[2],
        VALID_COMPONENTS,
      ),
    ).toBe(true);
    expect(useDockviewStore.getState().preMaximizeLayout?.columns[0]?.tree).toMatchObject({
      type: "leaf",
      group: {
        id: "g-center",
        panels: [{ id: activePanelId, params: { keep: "value", sessionId: activeSessionId } }],
        activePanel: activePanelId,
      },
    });

    useDockviewStore.getState().exitMaximizedLayout();
    await flushRaf();

    const persistedLayout = vi.mocked(persistLayout).mock.calls.at(-1)?.[1] as {
      panels?: Record<string, { params?: Record<string, string> }>;
    };
    expect(persistedLayout.panels).toHaveProperty(activePanelId);
    expect(persistedLayout.panels).not.toHaveProperty(`session:${staleSessionId}`);
    expect(persistedLayout.panels?.[activePanelId]?.params).toMatchObject({
      keep: "value",
      sessionId: activeSessionId,
    });
  });
});

describe("on-ready maximize restore with an incomplete session list", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });
  it("keeps unknown sibling sessions while the task session list is loading", async () => {
    const activeSessionId = "alive";
    const siblingSessionId = "loading-sibling";
    const knownPhantomSessionId = "known-phantom";
    const siblingPanelId = `session:${siblingSessionId}`;
    const phantomPanelId = `session:${knownPhantomSessionId}`;
    vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(makeSavedLayout());
    vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue({
      maximizedDockviewJson: makeMaximizeOverlay(`session:${activeSessionId}`),
      preMaximizeLayout: {
        columns: [
          {
            id: "center",
            groups: [],
            tree: {
              type: "leaf",
              size: 600,
              group: {
                id: "g-center",
                panels: [
                  {
                    id: siblingPanelId,
                    component: "chat",
                    title: "Sibling session",
                    params: { sessionId: siblingSessionId },
                  },
                  {
                    id: phantomPanelId,
                    component: "chat",
                    title: "Known phantom",
                    params: { sessionId: knownPhantomSessionId },
                  },
                ],
                activePanel: siblingPanelId,
              },
            },
          },
        ],
      },
    });
    let appliedLayout = {} as SerializedDockview;
    const api = makeRestoreApi();
    vi.mocked(api.fromJSON).mockImplementation((layout) => {
      appliedLayout = layout;
    });
    vi.mocked(api.toJSON).mockImplementation(() => appliedLayout);
    const persistLayout = vi.spyOn(localStorage, "setEnvLayout").mockReturnValue(true);
    const appStore = {
      getState: () => ({
        environmentIdBySessionId: {
          [activeSessionId]: "env-live",
          [knownPhantomSessionId]: "env-other",
        },
        tasks: { activeTaskId: "task-live", activeSessionId },
        taskSessionsByTask: {
          itemsByTaskId: { "task-live": [{ id: activeSessionId }] },
          loadedByTaskId: { "task-live": false },
        },
      }),
    };
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-live" });

    expect(
      restoreEnvLayout(
        api,
        "env-live",
        appStore as unknown as Parameters<typeof restoreEnvLayout>[2],
        VALID_COMPONENTS,
      ),
    ).toBe(true);

    expect(useDockviewStore.getState().preMaximizeLayout?.columns[0]?.tree).toMatchObject({
      type: "leaf",
      group: {
        id: "g-center",
        panels: [{ id: siblingPanelId, params: { sessionId: siblingSessionId } }],
        activePanel: siblingPanelId,
      },
    });

    useDockviewStore.getState().exitMaximizedLayout();
    await flushRaf();

    const persistedLayout = vi.mocked(persistLayout).mock.calls.at(-1)?.[1] as {
      panels?: Record<string, unknown>;
    };
    expect(persistedLayout.panels).toHaveProperty(siblingPanelId);
    expect(persistedLayout.panels).not.toHaveProperty(phantomPanelId);
  });
});
