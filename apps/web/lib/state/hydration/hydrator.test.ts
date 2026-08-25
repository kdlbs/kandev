/* eslint-disable max-lines -- hydration paths share one complete-state fixture. */
import { beforeEach, describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState, hydrateUI } from "./hydrator";
import { defaultUIState } from "@/lib/state/slices/ui/ui-slice";
import { defaultState, mergeInitialState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";

const TERMINAL_TAB_ID = "terminal-1";
const DEFAULT_TASK_ROW = {
  detailsEnabled: true,
  detailOrder: ["relative_time", "repository", "pull_request_number"],
  visibleDetails: ["relative_time", "repository", "pull_request_number"],
  trailing: "git_changes",
};

/** Builds an AppState draft carrying only the default UI slice for hydrateUI tests. */
function makeDraft(): AppState {
  // hydrateUI only touches UI-slice fields; an empty object cast satisfies
  // the rest without dragging the full AppState shape into this test.
  return { ...defaultUIState } as unknown as AppState;
}

/** Builds a deep-cloned AppState from the default state for full-state hydration tests. */
function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

describe("hydrateUI — quick chat name overlay", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("overlays a locally-renamed name onto the SSR-provided session name", () => {
    window.localStorage.setItem(
      "kandev.quickChat.names",
      JSON.stringify({ "sess-1": "My custom name" }),
    );

    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [
            { sessionId: "sess-1", workspaceId: "ws-1", name: "Agent A - Chat 1", kind: "chat" },
          ],
        },
      });
    });

    expect(result.quickChat.sessions[0].name).toBe("My custom name");
  });

  it("keeps the SSR-provided name when no local rename exists", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [
            { sessionId: "sess-2", workspaceId: "ws-1", name: "Agent A - Chat 1", kind: "chat" },
          ],
        },
      });
    });

    expect(result.quickChat.sessions[0].name).toBe("Agent A - Chat 1");
  });
});

describe("hydrateUI — typed quick chat sessions", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("normalizes a legacy session without kind to an ordinary chat", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [
            {
              sessionId: "legacy",
              workspaceId: "ws-1",
            } as unknown as (typeof draft.quickChat.sessions)[number],
          ],
        },
      });
    });

    expect((result.quickChat.sessions[0] as { kind?: string }).kind).toBe("chat");
  });

  it("preserves a restored configuration session kind", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [
            {
              sessionId: "config-session",
              workspaceId: "ws-1",
              kind: "config",
            } as (typeof draft.quickChat.sessions)[number],
          ],
        },
      });
    });

    expect((result.quickChat.sessions[0] as { kind?: string }).kind).toBe("config");
  });

  it("only overlays sessions that have a stored rename, leaving siblings untouched", () => {
    window.localStorage.setItem(
      "kandev.quickChat.names",
      JSON.stringify({ "sess-a": "Renamed A" }),
    );

    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [
            { sessionId: "sess-a", workspaceId: "ws-1", name: "Original A", kind: "chat" },
            { sessionId: "sess-b", workspaceId: "ws-1", name: "Original B", kind: "chat" },
          ],
        },
      });
    });

    expect(result.quickChat.sessions.map((s) => s.name)).toEqual(["Renamed A", "Original B"]);
  });
});

describe("hydrateUI — quick chat lifecycle", () => {
  it("restores server-owned terminal tabs during a fresh hydration", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [],
          terminalTabs: [
            {
              tabId: TERMINAL_TAB_ID,
              workspaceId: "ws-1",
              sessionId: "pty-1",
              sequence: 1,
              status: "running",
            },
          ],
          activeKind: "terminal",
          activeTerminalTabId: TERMINAL_TAB_ID,
          lastTerminalTabIdByWorkspace: { "ws-1": TERMINAL_TAB_ID },
        },
      });
    });

    expect(result.quickChat.terminalTabs).toEqual([
      expect.objectContaining({ tabId: TERMINAL_TAB_ID, sessionId: "pty-1" }),
    ]);
    expect(result.quickChat.activeKind).toBe("terminal");
    expect(result.quickChat.activeTerminalTabId).toBe(TERMINAL_TAB_ID);
  });

  it("preserves live quick chat sessions omitted by stale hydration", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      draft.quickChat = {
        ...draft.quickChat,
        isOpen: true,
        activeSessionId: "stale-session",
        sessions: [{ sessionId: "stale-session", workspaceId: "ws-1", name: "Live", kind: "chat" }],
      };
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [],
        },
      });
    });

    expect(result.quickChat.sessions).toHaveLength(1);
    expect(result.quickChat.sessions[0].sessionId).toBe("stale-session");
    expect(result.quickChat.isOpen).toBe(true);
  });

  it("preserves browser-local terminal tabs when server conversations resync", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      draft.quickChat = {
        ...draft.quickChat,
        isOpen: true,
        activeSessionId: "chat-1",
        sessions: [{ sessionId: "chat-1", workspaceId: "ws-1", kind: "chat" }],
        terminalTabs: [
          {
            tabId: TERMINAL_TAB_ID,
            workspaceId: "ws-1",
            sessionId: "pty-1",
            sequence: 1,
            status: "running",
          },
        ],
        activeKind: "terminal",
        activeTerminalTabId: TERMINAL_TAB_ID,
        lastTerminalTabIdByWorkspace: { "ws-1": TERMINAL_TAB_ID },
      };
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [],
        },
      });
    });

    expect(result.quickChat.sessions).toHaveLength(1);
    expect(result.quickChat.terminalTabs).toHaveLength(1);
    expect(result.quickChat.activeKind).toBe("terminal");
    expect(result.quickChat.activeTerminalTabId).toBe(TERMINAL_TAB_ID);
    expect(result.quickChat.isOpen).toBe(true);
  });
});

describe("hydrateUI — terminal descriptor reconciliation", () => {
  it("treats an empty server terminal list as authoritative during hydration", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      draft.quickChat = {
        ...draft.quickChat,
        isOpen: true,
        activeSessionId: null,
        sessions: [],
        terminalTabs: [
          {
            tabId: TERMINAL_TAB_ID,
            workspaceId: "ws-1",
            sessionId: "pty-stale",
            sequence: 1,
            status: "running",
          },
        ],
        activeKind: "terminal",
        activeTerminalTabId: TERMINAL_TAB_ID,
        lastTerminalTabIdByWorkspace: { "ws-1": TERMINAL_TAB_ID },
      };
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [],
          terminalTabs: [],
        },
      });
    });

    expect(result.quickChat.terminalTabs).toEqual([]);
    expect(result.quickChat.activeKind).toBe("conversation");
    expect(result.quickChat.activeTerminalTabId).toBeNull();
    expect(result.quickChat.isOpen).toBe(false);
  });
});

describe("mergeInitialState — quick chat name overlay", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("overlays locally-renamed names during boot payload merge", () => {
    window.localStorage.setItem(
      "kandev.quickChat.names",
      JSON.stringify({ "sess-boot": "Local boot name" }),
    );

    const result = mergeInitialState({
      quickChat: {
        isOpen: false,
        activeSessionId: null,
        sessions: [
          {
            sessionId: "sess-boot",
            workspaceId: "ws-1",
            name: "Backend task title",
            kind: "chat",
          },
          {
            sessionId: "sess-other",
            workspaceId: "ws-1",
            name: "Other title",
            kind: "chat",
          },
        ],
      },
    });

    expect(result.quickChat.sessions.map((s) => s.name)).toEqual([
      "Local boot name",
      "Other title",
    ]);
  });
});

describe("mergeInitialState — sidebar views from boot settings", () => {
  it("bridges backend sidebar settings into the UI slice before the store is created", () => {
    const result = mergeInitialState({
      userSettings: {
        sidebarViews: [
          {
            id: "server",
            name: "Server",
            filters: [],
            sort: { key: "state", direction: "asc" },
            group: "none",
            collapsedGroups: [],
          },
        ],
        sidebarActiveViewId: "server",
        sidebarDraft: {
          baseViewId: "server",
          filters: [],
          sort: { key: "updatedAt", direction: "desc" },
          group: "workflow",
        },
        loaded: true,
      },
    } as unknown as Partial<AppState>);

    expect(result.sidebarViews).toMatchObject({
      views: [{ id: "server", name: "Server" }],
      activeViewId: "server",
      draft: { baseViewId: "server", group: "workflow" },
    });
  });

  it("bridges backend task preferences into the UI slice before the store is created", () => {
    const result = mergeInitialState({
      userSettings: {
        sidebarTaskPrefs: {
          pinnedTaskIds: ["task-1"],
          orderedTaskIds: ["task-2", "task-1"],
          subtaskOrderByParentId: { "task-1": ["subtask-1"] },
        },
        loaded: true,
      },
    } as unknown as Partial<AppState>);

    expect(result.sidebarTaskPrefs).toMatchObject({
      pinnedTaskIds: ["task-1"],
      orderedTaskIds: ["task-2", "task-1"],
      subtaskOrderByParentId: { "task-1": ["subtask-1"] },
    });
  });

  it("preserves an archived clause in the boot draft", () => {
    const result = mergeInitialState({
      userSettings: {
        sidebarDraft: {
          baseViewId: "server",
          filters: [
            { id: "archived", dimension: "archived", op: "is", value: true },
            { id: "title", dimension: "titleMatch", op: "matches", value: "keep" },
          ],
          sort: { key: "state", direction: "asc" },
          group: "none",
        },
        loaded: true,
      },
    } as unknown as Partial<AppState>);

    expect(result.sidebarViews.draft?.filters).toEqual([
      { id: "archived", dimension: "archived", op: "is", value: true },
      { id: "title", dimension: "titleMatch", op: "matches", value: "keep" },
    ]);
  });
});

describe("hydrateState — user settings revisions", () => {
  it("applies a newer boot snapshot after an earlier websocket update", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.userSettings.loaded = true;
      draft.userSettings.revision = 1;
      draft.userSettings.appStatusBarEnabled = false;
      hydrateState(draft, {
        userSettings: {
          loaded: true,
          revision: 2,
          appStatusBarEnabled: true,
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.userSettings).toMatchObject({
      loaded: true,
      revision: 2,
      appStatusBarEnabled: true,
    });
  });

  it("keeps a newer websocket snapshot when boot hydration is older", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.userSettings.loaded = true;
      draft.userSettings.revision = 3;
      draft.userSettings.appStatusBarEnabled = true;
      hydrateState(draft, {
        userSettings: {
          loaded: true,
          revision: 2,
          appStatusBarEnabled: false,
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.userSettings).toMatchObject({
      revision: 3,
      appStatusBarEnabled: true,
    });
  });
});

describe("hydrateState — sidebar views from user settings", () => {
  it("hydrates active view and draft from backend user settings", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sidebarViews.activeViewId = "local";
      hydrateState(draft, {
        userSettings: {
          sidebarViews: [
            {
              id: "server",
              name: "Server",
              filters: [],
              sort: { key: "state", direction: "asc" },
              group: "none",
              collapsedGroups: [],
            },
          ],
          sidebarActiveViewId: "server",
          sidebarDraft: {
            baseViewId: "server",
            filters: [],
            sort: { key: "updatedAt", direction: "desc" },
            group: "workflow",
          },
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.sidebarViews.activeViewId).toBe("server");
    expect(result.sidebarViews.draft).toEqual({
      baseViewId: "server",
      filters: [],
      sort: { key: "updatedAt", direction: "desc" },
      group: "workflow",
      taskRow: DEFAULT_TASK_ROW,
    });
  });

  it("clears stale local draft when backend draft is null", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sidebarViews.draft = {
        baseViewId: "local",
        filters: [],
        sort: { key: "state", direction: "asc" },
        group: "state",
      };
      hydrateState(draft, {
        userSettings: {
          sidebarDraft: null,
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.sidebarViews.draft).toBeNull();
  });

  it("hydrates sidebar task prefs from backend, including explicit clears", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sidebarTaskPrefs = {
        pinnedTaskIds: ["local-pin"],
        orderedTaskIds: ["local-order"],
        subtaskOrderByParentId: { parent: ["child"] },
      };
      hydrateState(draft, {
        userSettings: {
          sidebarTaskPrefs: {
            pinnedTaskIds: [],
            orderedTaskIds: [],
            subtaskOrderByParentId: {},
          },
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.sidebarTaskPrefs).toEqual({
      pinnedTaskIds: [],
      orderedTaskIds: [],
      subtaskOrderByParentId: {},
    });
  });

  it("uses backend sidebar task prefs as the authoritative hydrated value", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sidebarTaskPrefs = {
        pinnedTaskIds: ["local-pin"],
        orderedTaskIds: ["local-order"],
        subtaskOrderByParentId: { shared: ["local-child"], localOnly: ["child"] },
        syncError: "retry",
      };
      hydrateState(draft, {
        userSettings: {
          sidebarTaskPrefs: {
            pinnedTaskIds: ["server-pin"],
            orderedTaskIds: ["server-order"],
            subtaskOrderByParentId: { shared: ["server-child"], serverOnly: ["child"] },
          },
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.sidebarTaskPrefs).toEqual({
      pinnedTaskIds: ["server-pin"],
      orderedTaskIds: ["server-order"],
      subtaskOrderByParentId: { shared: ["server-child"], serverOnly: ["child"] },
      syncError: "retry",
    });
  });
});

describe("hydrateState — sidebar draft migration", () => {
  it("preserves an archived clause during hydration", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {
        userSettings: {
          sidebarDraft: {
            baseViewId: "server",
            filters: [{ id: "archived", dimension: "archived", op: "is", value: true }],
            sort: { key: "state", direction: "asc" },
            group: "none",
          },
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.sidebarViews.draft?.filters).toEqual([
      { id: "archived", dimension: "archived", op: "is", value: true },
    ]);
  });
});

describe("hydrateState — session runtime model state", () => {
  const sessionId = "session-1";
  const modelId = "claude-opus-4-8";
  const liveModelId = "live-model";

  it("force-merges sessionModels over live state so the selector survives resume", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sessionModels.bySessionId[sessionId] = {
        currentModelId: liveModelId,
        models: [{ modelId: liveModelId, name: "Live" }],
        configOptions: [],
      };
      hydrateState(
        draft,
        {
          sessionModels: {
            bySessionId: {
              [sessionId]: {
                currentModelId: modelId,
                models: [{ modelId, name: "Opus" }],
                configOptions: [],
              },
            },
          },
        } as unknown as Partial<AppState>,
        { activeSessionId: sessionId, forceMergeSessionId: sessionId },
      );
    });

    expect(result.sessionModels.bySessionId[sessionId]).toEqual({
      currentModelId: modelId,
      models: [{ modelId, name: "Opus" }],
      configOptions: [],
    });
  });

  it("force-merges sessionMcpStatus over live state", () => {
    const bootHistory: MCPAttachmentHistory = {
      version: 1,
      current: {
        attachment_attempt_id: "attempt-boot",
        started_at: "2026-06-11T00:00:00.000Z",
        servers: [{ name: "fs", status: "connected" }],
      },
    };
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sessionMcpStatus.bySessionId[sessionId] = {
        version: 1,
        current: {
          attachment_attempt_id: "attempt-live",
          started_at: "2026-06-10T00:00:00.000Z",
          servers: [{ name: "stale", status: "failed" }],
        },
      };
      hydrateState(
        draft,
        {
          sessionMcpStatus: { bySessionId: { [sessionId]: bootHistory } },
        } as unknown as Partial<AppState>,
        { activeSessionId: sessionId, forceMergeSessionId: sessionId },
      );
    });

    expect(result.sessionMcpStatus.bySessionId[sessionId]).toEqual(bootHistory);
  });

  it("does not overwrite live model state for the active (non-force-merged) session", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.sessionModels.bySessionId["active-session"] = {
        currentModelId: liveModelId,
        models: [],
        configOptions: [],
      };
      hydrateState(
        draft,
        {
          sessionModels: {
            bySessionId: {
              "active-session": {
                currentModelId: "stale-model",
                models: [],
                configOptions: [],
              },
            },
          },
        } as unknown as Partial<AppState>,
        { activeSessionId: "active-session" },
      );
    });

    expect(result.sessionModels.bySessionId["active-session"].currentModelId).toBe(liveModelId);

    const systemResult = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {});
    });
    expect(systemResult.system).toEqual(defaultState.system);
  });
});
it.each([true, false])(
  "turn hydration marks a session only when it is force-merged",
  (forceMerge) => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      if (!forceMerge) draft.turns.bySession["session-1"] = [];
      hydrateState(
        draft,
        { turns: { bySession: { "session-1": [] } } } as unknown as Partial<AppState>,
        { activeSessionId: "session-1", forceMergeSessionId: forceMerge ? "session-1" : null },
      );
    });

    expect(result.turns.loadedBySession).toEqual(forceMerge ? { "session-1": true } : {});
  },
);

it("marks an absent inactive session after ordinary turn hydration", () => {
  const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
    hydrateState(
      draft,
      { turns: { bySession: { "session-1": [] } } } as unknown as Partial<AppState>,
      { activeSessionId: "other-session", forceMergeSessionId: null },
    );
  });

  expect(result.turns.loadedBySession["session-1"]).toBe(true);
});
