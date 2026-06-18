import { beforeEach, describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState, hydrateUI } from "./hydrator";
import { defaultUIState } from "@/lib/state/slices/ui/ui-slice";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

function makeDraft(): AppState {
  // hydrateUI only touches UI-slice fields; an empty object cast satisfies
  // the rest without dragging the full AppState shape into this test.
  return { ...defaultUIState } as unknown as AppState;
}

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
          sessions: [{ sessionId: "sess-1", workspaceId: "ws-1", name: "Agent A - Chat 1" }],
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
          sessions: [{ sessionId: "sess-2", workspaceId: "ws-1", name: "Agent A - Chat 1" }],
        },
      });
    });

    expect(result.quickChat.sessions[0].name).toBe("Agent A - Chat 1");
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
            { sessionId: "sess-a", workspaceId: "ws-1", name: "Original A" },
            { sessionId: "sess-b", workspaceId: "ws-1", name: "Original B" },
          ],
        },
      });
    });

    expect(result.quickChat.sessions.map((s) => s.name)).toEqual(["Renamed A", "Original B"]);
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

  it("does not replace local task prefs with empty backend defaults during first hydration", () => {
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
      pinnedTaskIds: ["local-pin"],
      orderedTaskIds: ["local-order"],
      subtaskOrderByParentId: { parent: ["child"] },
    });
  });
});
