import type { StateCreator } from "zustand";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  getStoredCollapsedSubtaskParents,
  getStoredSidebarActiveViewId,
  getStoredSidebarDraft,
  getStoredSidebarUserViews,
  removeStoredSidebarDraft,
  setLocalStorage,
  setStoredCollapsedSubtaskParents,
  setStoredSidebarActiveViewId,
  setStoredSidebarDraft,
  setStoredSidebarUserViews,
} from "@/lib/local-storage";
import { DEFAULT_ACTIVE_VIEW_ID, DEFAULT_VIEW } from "./sidebar-view-builtins";
import type {
  FilterClause,
  GroupKey,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "./sidebar-view-types";
import { toApiSidebarView } from "./sidebar-view-wire";
import type { SystemHealthResponse } from "@/lib/types/health";
import type { ActiveDocument, UISlice, UISliceState } from "./types";

function loadSidebarState(): UISliceState["sidebarViews"] {
  let views = getStoredSidebarUserViews<SidebarView[]>([]).map(migrateView);
  if (views.length === 0) {
    views = [DEFAULT_VIEW];
  }
  setStoredSidebarUserViews(views);
  const storedActive = getStoredSidebarActiveViewId(DEFAULT_ACTIVE_VIEW_ID);
  const activeViewId = views.some((v) => v.id === storedActive) ? storedActive : views[0].id;
  const draft = getStoredSidebarDraft<SidebarViewDraft | null>(null);
  return { views, activeViewId, draft, syncError: null };
}

export const KNOWN_DIMENSIONS = new Set<string>([
  "archived",
  "state",
  "workflow",
  "workflowStep",
  "executorType",
  "repository",
  "hasDiff",
  "isPRReview",
  "titleMatch",
]);

export const KNOWN_SORT_KEYS = new Set<string>(["state", "updatedAt", "createdAt", "title"]);

// Drops clauses whose dimension is no longer known (e.g. renamed or removed in an upgrade),
// and resets stale sort keys, so the popover does not crash when rendering stored views.
export function migrateView(view: SidebarView): SidebarView {
  const sort: SortSpec = KNOWN_SORT_KEYS.has(view.sort.key)
    ? view.sort
    : { key: "state", direction: view.sort.direction };
  return {
    ...view,
    filters: view.filters.filter((c) => KNOWN_DIMENSIONS.has(c.dimension)),
    sort,
  };
}

function persistUserViews(views: SidebarView[]): void {
  setStoredSidebarUserViews(views);
}

function makeId(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export const defaultUIState: UISliceState = {
  previewPanel: {
    openBySessionId: {},
    viewBySessionId: {},
    deviceBySessionId: {},
    stageBySessionId: {},
    urlBySessionId: {},
    urlDraftBySessionId: {},
  },
  rightPanel: { activeTabBySessionId: {} },
  diffs: { files: [] },
  connection: { status: "disconnected", error: null },
  mobileKanban: { activeColumnIndex: 0, isMenuOpen: false },
  mobileSession: { activePanelBySessionId: {}, isTaskSwitcherOpen: false },
  chatInput: { planModeBySessionId: {} },
  documentPanel: { activeDocumentBySessionId: {} },
  systemHealth: { issues: [], healthy: true, loaded: false, loading: false },
  quickChat: { isOpen: false, sessions: [], activeSessionId: null },
  configChat: { isOpen: false, sessions: [], activeSessionId: null, workspaceId: null },
  sessionFailureNotification: null,
  bottomTerminal: { isOpen: false, pendingCommand: null },
  sidebarViews: loadSidebarState(),
  collapsedSubtaskParents: [],
};

type ImmerSet = Parameters<typeof createUISlice>[0];

function buildPreviewActions(set: ImmerSet) {
  return {
    setPreviewOpen: (sessionId: string, open: boolean) =>
      set((draft) => {
        draft.previewPanel.openBySessionId[sessionId] = open;
        setLocalStorage(`preview-open-${sessionId}`, open);
      }),
    togglePreviewOpen: (sessionId: string) =>
      set((draft) => {
        const current = draft.previewPanel.openBySessionId[sessionId] ?? false;
        draft.previewPanel.openBySessionId[sessionId] = !current;
        setLocalStorage(`preview-open-${sessionId}`, !current);
      }),
    setPreviewView: (
      sessionId: string,
      view: UISliceState["previewPanel"]["viewBySessionId"][string],
    ) =>
      set((draft) => {
        draft.previewPanel.viewBySessionId[sessionId] = view;
        setLocalStorage(`preview-view-${sessionId}`, view);
      }),
    setPreviewDevice: (
      sessionId: string,
      device: UISliceState["previewPanel"]["deviceBySessionId"][string],
    ) =>
      set((draft) => {
        draft.previewPanel.deviceBySessionId[sessionId] = device;
        setLocalStorage(`preview-device-${sessionId}`, device);
      }),
    setPreviewStage: (
      sessionId: string,
      stage: UISliceState["previewPanel"]["stageBySessionId"][string],
    ) =>
      set((draft) => {
        draft.previewPanel.stageBySessionId[sessionId] = stage;
      }),
    setPreviewUrl: (sessionId: string, url: string) =>
      set((draft) => {
        draft.previewPanel.urlBySessionId[sessionId] = url;
      }),
    setPreviewUrlDraft: (sessionId: string, url: string) =>
      set((draft) => {
        draft.previewPanel.urlDraftBySessionId[sessionId] = url;
      }),
  };
}

function buildMobileActions(set: ImmerSet) {
  return {
    setMobileKanbanColumnIndex: (index: number) =>
      set((draft) => {
        draft.mobileKanban.activeColumnIndex = index;
      }),
    setMobileKanbanMenuOpen: (open: boolean) =>
      set((draft) => {
        draft.mobileKanban.isMenuOpen = open;
      }),
    setMobileSessionPanel: (
      sessionId: string,
      panel: UISliceState["mobileSession"]["activePanelBySessionId"][string],
    ) =>
      set((draft) => {
        draft.mobileSession.activePanelBySessionId[sessionId] = panel;
      }),
    setMobileSessionTaskSwitcherOpen: (open: boolean) =>
      set((draft) => {
        draft.mobileSession.isTaskSwitcherOpen = open;
      }),
  };
}

function buildBottomTerminalActions(set: ImmerSet) {
  return {
    toggleBottomTerminal: () =>
      set((draft) => {
        const newValue = !draft.bottomTerminal.isOpen;
        draft.bottomTerminal.isOpen = newValue;
        setLocalStorage("bottom-terminal-open", String(newValue));
      }),
    openBottomTerminalWithCommand: (command: string) =>
      set((draft) => {
        draft.bottomTerminal.isOpen = true;
        draft.bottomTerminal.pendingCommand = command;
        setLocalStorage("bottom-terminal-open", "true");
      }),
    clearBottomTerminalCommand: () =>
      set((draft) => {
        draft.bottomTerminal.pendingCommand = null;
      }),
  };
}

// Tracks the most recent in-flight views PATCH. On failure, only the latest
// request is allowed to revert — earlier failed requests are ignored because
// their state is already stale (a newer action is in flight).
let viewsSyncRequestId = 0;

type SidebarSnapshot = {
  views: SidebarView[];
  activeViewId: string;
  draft: SidebarViewDraft | null;
};

function snapshotSidebar(s: UISliceState["sidebarViews"]): SidebarSnapshot {
  return {
    views: s.views.map(cloneView),
    activeViewId: s.activeViewId,
    draft: s.draft ? { ...s.draft } : null,
  };
}

function writeCacheFromSidebar(s: SidebarSnapshot | UISliceState["sidebarViews"]) {
  persistUserViews(s.views);
  setStoredSidebarActiveViewId(s.activeViewId);
  if (s.draft) setStoredSidebarDraft(s.draft);
  else removeStoredSidebarDraft();
}

function mutateViews(
  set: ImmerSet,
  get: () => UISlice,
  mutate: (slice: UISliceState["sidebarViews"]) => boolean | void,
): void {
  const snapshot = snapshotSidebar(get().sidebarViews);
  let committed = false;
  set((draft) => {
    committed = mutate(draft.sidebarViews) !== false;
  });
  if (!committed) return;
  const after = get().sidebarViews;
  writeCacheFromSidebar(after);
  const thisRequestId = ++viewsSyncRequestId;
  updateUserSettings({ sidebar_views: after.views.map(toApiSidebarView) }).catch((err) => {
    if (thisRequestId !== viewsSyncRequestId) return;
    const message = err instanceof Error ? err.message : "Failed to sync sidebar views";
    set((draft) => {
      draft.sidebarViews.views = snapshot.views;
      draft.sidebarViews.activeViewId = snapshot.activeViewId;
      draft.sidebarViews.draft = snapshot.draft;
      draft.sidebarViews.syncError = message;
    });
    writeCacheFromSidebar(snapshot);
  });
}

function buildSidebarLocalActions(set: ImmerSet, get: () => UISlice) {
  return {
    setSidebarActiveView: (viewId: string) =>
      set((draft) => {
        if (!draft.sidebarViews.views.some((v) => v.id === viewId)) return;
        draft.sidebarViews.activeViewId = viewId;
        draft.sidebarViews.draft = null;
        setStoredSidebarActiveViewId(viewId);
        removeStoredSidebarDraft();
      }),
    updateSidebarDraft: (
      patch: Partial<{ filters: FilterClause[]; sort: SortSpec; group: GroupKey }>,
    ) =>
      set((draft) => {
        const active = draft.sidebarViews.views.find(
          (v) => v.id === draft.sidebarViews.activeViewId,
        );
        if (!active) return;
        const current: SidebarViewDraft = draft.sidebarViews.draft ?? {
          baseViewId: active.id,
          filters: active.filters,
          sort: active.sort,
          group: active.group,
        };
        const next: SidebarViewDraft = {
          baseViewId: active.id,
          filters: patch.filters ?? current.filters,
          sort: patch.sort ?? current.sort,
          group: patch.group ?? current.group,
        };
        draft.sidebarViews.draft = next;
        setStoredSidebarDraft(next);
      }),
    discardSidebarDraft: () =>
      set((draft) => {
        draft.sidebarViews.draft = null;
        removeStoredSidebarDraft();
      }),
    clearSidebarSyncError: () =>
      set((draft) => {
        draft.sidebarViews.syncError = null;
      }),
    // collapsedGroups is per-device visual state; update in memory and the
    // localStorage cache only. Don't PATCH — we'd flood the backend on every
    // expand/collapse click, and the server-side copy is stale anyway across
    // devices. The next mutateViews call picks up any newer local state.
    toggleSidebarGroupCollapsed: (viewId: string, groupKey: string) => {
      set((draft) => {
        const view = draft.sidebarViews.views.find((v) => v.id === viewId);
        if (!view) return;
        const idx = view.collapsedGroups.indexOf(groupKey);
        if (idx === -1) view.collapsedGroups.push(groupKey);
        else view.collapsedGroups.splice(idx, 1);
      });
      persistUserViews(get().sidebarViews.views);
    },
    migrateLocalViewsToBackend: () => {
      const views = get().sidebarViews.views;
      const thisRequestId = ++viewsSyncRequestId;
      updateUserSettings({ sidebar_views: views.map(toApiSidebarView) }).catch((err) => {
        if (thisRequestId !== viewsSyncRequestId) return;
        const message = err instanceof Error ? err.message : "Failed to sync sidebar views";
        set((draft) => {
          draft.sidebarViews.syncError = message;
        });
      });
    },
  };
}

function buildSidebarBackendActions(set: ImmerSet, get: () => UISlice) {
  const mv = (mutate: (s: UISliceState["sidebarViews"]) => boolean | void) =>
    mutateViews(set, get, mutate);
  return {
    saveSidebarDraftAs: (name: string) =>
      mv((s) => {
        if (!s.draft) return false;
        s.views.push({
          id: makeId("view"),
          name: name.trim() || "Untitled view",
          filters: s.draft.filters,
          sort: s.draft.sort,
          group: s.draft.group,
          collapsedGroups: [],
        });
        s.activeViewId = s.views[s.views.length - 1].id;
        s.draft = null;
      }),
    saveSidebarDraftOverwrite: () =>
      mv((s) => {
        if (!s.draft) return false;
        const view = s.views.find((v) => v.id === s.draft!.baseViewId);
        if (!view) return false;
        view.filters = s.draft.filters;
        view.sort = s.draft.sort;
        view.group = s.draft.group;
        s.draft = null;
      }),
    duplicateSidebarView: (viewId: string, name: string) =>
      mv((s) => {
        const source = s.views.find((v) => v.id === viewId);
        if (!source) return false;
        s.views.push({
          id: makeId("view"),
          name: name.trim() || `${source.name} copy`,
          filters: source.filters.map((f) => ({ ...f, id: makeId("clause") })),
          sort: source.sort,
          group: source.group,
          collapsedGroups: [],
        });
        s.activeViewId = s.views[s.views.length - 1].id;
      }),
    deleteSidebarView: (viewId: string) =>
      mv((s) => {
        const remaining = s.views.filter((v) => v.id !== viewId);
        if (remaining.length === 0) return false;
        s.views = remaining;
        if (s.activeViewId === viewId) s.activeViewId = remaining[0].id;
        s.draft = null;
      }),
    renameSidebarView: (viewId: string, name: string) =>
      mv((s) => {
        const view = s.views.find((v) => v.id === viewId);
        if (!view) return false;
        const next = name.trim();
        if (!next || next === view.name) return false;
        view.name = next;
      }),
  };
}

function buildSidebarViewActions(set: ImmerSet, get: () => UISlice) {
  return {
    ...buildSidebarLocalActions(set, get),
    ...buildSidebarBackendActions(set, get),
  };
}

function cloneView(v: SidebarView): SidebarView {
  return {
    id: v.id,
    name: v.name,
    filters: v.filters.map((f) => ({ ...f })),
    sort: { ...v.sort },
    group: v.group,
    collapsedGroups: [...v.collapsedGroups],
  };
}

function buildSystemHealthActions(set: ImmerSet) {
  return {
    setSystemHealth: (response: SystemHealthResponse) =>
      set((draft) => {
        draft.systemHealth.issues = response.issues;
        draft.systemHealth.healthy = response.healthy;
        draft.systemHealth.loaded = true;
      }),
    setSystemHealthLoading: (loading: boolean) =>
      set((draft) => {
        draft.systemHealth.loading = loading;
      }),
    invalidateSystemHealth: () =>
      set((draft) => {
        draft.systemHealth.loaded = false;
      }),
  };
}

function buildCollapsedSubtaskActions(set: ImmerSet, get: () => UISlice) {
  return {
    // Tab-scoped collapse of a parent task's subtasks. Persisted via
    // sessionStorage (survives reload / task switch within the tab, resets on
    // tab close). Not per-view and not synced to the backend — purely visual.
    toggleSubtaskCollapsed: (parentTaskId: string) => {
      set((draft) => {
        const list = draft.collapsedSubtaskParents;
        const idx = list.indexOf(parentTaskId);
        if (idx === -1) list.push(parentTaskId);
        else list.splice(idx, 1);
      });
      setStoredCollapsedSubtaskParents(get().collapsedSubtaskParents);
    },
  };
}

function buildConfigChatActions(set: ImmerSet) {
  return {
    openConfigChat: (sessionId: string, workspaceId: string) =>
      set((draft) => {
        draft.configChat.isOpen = true;
        draft.configChat.workspaceId = workspaceId;
        const exists = draft.configChat.sessions.some((s) => s.sessionId === sessionId);
        if (!exists) {
          draft.configChat.sessions.push({ sessionId, workspaceId });
        }
        draft.configChat.activeSessionId = sessionId;
      }),
    startNewConfigChat: (workspaceId: string) =>
      set((draft) => {
        draft.configChat.isOpen = true;
        draft.configChat.activeSessionId = null;
        draft.configChat.workspaceId = workspaceId;
      }),
    closeConfigChat: () =>
      set((draft) => {
        draft.configChat.isOpen = false;
      }),
    closeConfigChatSession: (sessionId: string) =>
      set((draft) => {
        draft.configChat.sessions = draft.configChat.sessions.filter(
          (s) => s.sessionId !== sessionId,
        );
        if (draft.configChat.activeSessionId === sessionId) {
          if (draft.configChat.sessions.length > 0) {
            const next = draft.configChat.sessions[0];
            draft.configChat.activeSessionId = next.sessionId;
            draft.configChat.workspaceId = next.workspaceId;
          } else {
            draft.configChat.activeSessionId = null;
            draft.configChat.workspaceId = null;
          }
        }
      }),
    setActiveConfigChatSession: (sessionId: string) =>
      set((draft) => {
        draft.configChat.activeSessionId = sessionId;
      }),
    renameConfigChatSession: (sessionId: string, name: string) =>
      set((draft) => {
        const session = draft.configChat.sessions.find((s) => s.sessionId === sessionId);
        if (session) {
          session.name = name;
        }
      }),
  };
}

export const createUISlice: StateCreator<UISlice, [["zustand/immer", never]], [], UISlice> = (
  set,
  get,
) => ({
  ...defaultUIState,
  // Hydrate from sessionStorage at slice creation (runs in the browser, after
  // the default static state) so tests and SSR both see a fresh read.
  collapsedSubtaskParents: getStoredCollapsedSubtaskParents(),
  ...buildPreviewActions(set),
  ...buildMobileActions(set),
  ...buildBottomTerminalActions(set),
  ...buildConfigChatActions(set),
  ...buildSidebarViewActions(set, get),
  ...buildCollapsedSubtaskActions(set, get),
  ...buildSystemHealthActions(set),
  setRightPanelActiveTab: (sessionId, tab) =>
    set((draft) => {
      draft.rightPanel.activeTabBySessionId[sessionId] = tab;
    }),
  setConnectionStatus: (status, error) =>
    set((draft) => {
      draft.connection.status = status;
      draft.connection.error = error ?? null;
    }),
  setPlanMode: (sessionId, enabled) =>
    set((draft) => {
      draft.chatInput.planModeBySessionId[sessionId] = enabled;
      setLocalStorage(`plan-mode-${sessionId}`, enabled);
    }),
  setActiveDocument: (sessionId, doc) =>
    set((draft) => {
      draft.documentPanel.activeDocumentBySessionId[sessionId] = doc;
      setLocalStorage(`active-document-${sessionId}`, doc as ActiveDocument | null);
    }),
  openQuickChat: (sessionId, workspaceId, agentProfileId) =>
    set((draft) => {
      draft.quickChat.isOpen = true;
      // If sessionId is empty, create a placeholder tab for agent selection
      if (!sessionId) {
        // Check if there's already an empty tab
        const emptyTabExists = draft.quickChat.sessions.some((s) => s.sessionId === "");
        if (!emptyTabExists) {
          draft.quickChat.sessions.push({ sessionId: "", workspaceId });
        }
        draft.quickChat.activeSessionId = "";
        return;
      }
      const existing = draft.quickChat.sessions.find((s) => s.sessionId === sessionId);
      if (existing) {
        if (agentProfileId) existing.agentProfileId = agentProfileId;
      } else {
        draft.quickChat.sessions.push({ sessionId, workspaceId, agentProfileId });
      }
      draft.quickChat.activeSessionId = sessionId;
    }),
  closeQuickChat: () =>
    set((draft) => {
      draft.quickChat.isOpen = false;
    }),
  closeQuickChatSession: (sessionId) =>
    set((draft) => {
      // Remove session from list
      draft.quickChat.sessions = draft.quickChat.sessions.filter((s) => s.sessionId !== sessionId);
      // If closing active session, switch to another or close modal
      if (draft.quickChat.activeSessionId === sessionId) {
        if (draft.quickChat.sessions.length > 0) {
          draft.quickChat.activeSessionId = draft.quickChat.sessions[0].sessionId;
        } else {
          draft.quickChat.activeSessionId = null;
          draft.quickChat.isOpen = false;
        }
      }
    }),
  setActiveQuickChatSession: (sessionId) =>
    set((draft) => {
      draft.quickChat.activeSessionId = sessionId;
    }),
  renameQuickChatSession: (sessionId, name) =>
    set((draft) => {
      const session = draft.quickChat.sessions.find((s) => s.sessionId === sessionId);
      if (session) {
        session.name = name;
      }
    }),
  setSessionFailureNotification: (n) =>
    set((draft) => {
      draft.sessionFailureNotification = n;
    }),
});
