import type { StateCreator } from "zustand";
import {
  getStoredSidebarActiveViewId,
  getStoredSidebarDraft,
  getStoredSidebarUserViews,
  removeStoredSidebarDraft,
  setLocalStorage,
  setStoredSidebarActiveViewId,
  setStoredSidebarDraft,
  setStoredSidebarUserViews,
} from "@/lib/local-storage";
import { BUILTIN_VIEWS, DEFAULT_ACTIVE_VIEW_ID } from "./sidebar-view-builtins";
import type {
  FilterClause,
  GroupKey,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "./sidebar-view-types";
import type { ActiveDocument, UISlice, UISliceState } from "./types";

function loadSidebarState(): UISliceState["sidebarViews"] {
  const userViews = getStoredSidebarUserViews<SidebarView[]>([]);
  const views = [...BUILTIN_VIEWS, ...userViews.filter((v) => !v.isBuiltIn)];
  const storedActive = getStoredSidebarActiveViewId(DEFAULT_ACTIVE_VIEW_ID);
  const activeViewId = views.some((v) => v.id === storedActive)
    ? storedActive
    : DEFAULT_ACTIVE_VIEW_ID;
  const draft = getStoredSidebarDraft<SidebarViewDraft | null>(null);
  return { views, activeViewId, draft };
}

function persistUserViews(views: SidebarView[]): void {
  setStoredSidebarUserViews(views.filter((v) => !v.isBuiltIn) as unknown as SidebarView[]);
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

function buildSidebarDraftActions(set: ImmerSet) {
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
        setStoredSidebarDraft(next as never);
      }),
    discardSidebarDraft: () =>
      set((draft) => {
        draft.sidebarViews.draft = null;
        removeStoredSidebarDraft();
      }),
  };
}

function buildSidebarViewSaveActions(set: ImmerSet) {
  return {
    saveSidebarDraftAs: (name: string) =>
      set((draft) => {
        const d = draft.sidebarViews.draft;
        if (!d) return;
        const newView: SidebarView = {
          id: makeId("view"),
          name: name.trim() || "Untitled view",
          filters: d.filters,
          sort: d.sort,
          group: d.group,
          collapsedGroups: [],
        };
        draft.sidebarViews.views.push(newView);
        draft.sidebarViews.activeViewId = newView.id;
        draft.sidebarViews.draft = null;
        persistUserViews(draft.sidebarViews.views);
        setStoredSidebarActiveViewId(newView.id);
        removeStoredSidebarDraft();
      }),
    saveSidebarDraftOverwrite: () =>
      set((draft) => {
        const d = draft.sidebarViews.draft;
        if (!d) return;
        const view = draft.sidebarViews.views.find((v) => v.id === d.baseViewId);
        if (!view || view.isBuiltIn) return;
        view.filters = d.filters;
        view.sort = d.sort;
        view.group = d.group;
        draft.sidebarViews.draft = null;
        persistUserViews(draft.sidebarViews.views);
        removeStoredSidebarDraft();
      }),
    duplicateSidebarView: (viewId: string, name: string) =>
      set((draft) => {
        const source = draft.sidebarViews.views.find((v) => v.id === viewId);
        if (!source) return;
        const copy: SidebarView = {
          id: makeId("view"),
          name: name.trim() || `${source.name} copy`,
          filters: source.filters.map((f) => ({ ...f, id: makeId("clause") })),
          sort: source.sort,
          group: source.group,
          collapsedGroups: [],
        };
        draft.sidebarViews.views.push(copy);
        draft.sidebarViews.activeViewId = copy.id;
        persistUserViews(draft.sidebarViews.views);
        setStoredSidebarActiveViewId(copy.id);
      }),
  };
}

function buildSidebarViewEditActions(set: ImmerSet) {
  return {
    deleteSidebarView: (viewId: string) =>
      set((draft) => {
        const view = draft.sidebarViews.views.find((v) => v.id === viewId);
        if (!view || view.isBuiltIn) return;
        draft.sidebarViews.views = draft.sidebarViews.views.filter((v) => v.id !== viewId);
        if (draft.sidebarViews.activeViewId === viewId) {
          draft.sidebarViews.activeViewId = DEFAULT_ACTIVE_VIEW_ID;
          setStoredSidebarActiveViewId(DEFAULT_ACTIVE_VIEW_ID);
        }
        persistUserViews(draft.sidebarViews.views);
      }),
    renameSidebarView: (viewId: string, name: string) =>
      set((draft) => {
        const view = draft.sidebarViews.views.find((v) => v.id === viewId);
        if (!view || view.isBuiltIn) return;
        view.name = name.trim() || view.name;
        persistUserViews(draft.sidebarViews.views);
      }),
    toggleSidebarGroupCollapsed: (viewId: string, groupKey: string) =>
      set((draft) => {
        const view = draft.sidebarViews.views.find((v) => v.id === viewId);
        if (!view) return;
        const idx = view.collapsedGroups.indexOf(groupKey);
        if (idx === -1) view.collapsedGroups.push(groupKey);
        else view.collapsedGroups.splice(idx, 1);
        if (!view.isBuiltIn) persistUserViews(draft.sidebarViews.views);
      }),
  };
}

function buildSidebarViewActions(set: ImmerSet) {
  return {
    ...buildSidebarDraftActions(set),
    ...buildSidebarViewSaveActions(set),
    ...buildSidebarViewEditActions(set),
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
) => ({
  ...defaultUIState,
  ...buildPreviewActions(set),
  ...buildMobileActions(set),
  ...buildBottomTerminalActions(set),
  ...buildConfigChatActions(set),
  ...buildSidebarViewActions(set),
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
  setSystemHealth: (response) =>
    set((draft) => {
      draft.systemHealth.issues = response.issues;
      draft.systemHealth.healthy = response.healthy;
      draft.systemHealth.loaded = true;
    }),
  setSystemHealthLoading: (loading) =>
    set((draft) => {
      draft.systemHealth.loading = loading;
    }),
  invalidateSystemHealth: () =>
    set((draft) => {
      draft.systemHealth.loaded = false;
    }),
  openQuickChat: (sessionId, workspaceId) =>
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
      // Add session if not already in list
      const exists = draft.quickChat.sessions.some((s) => s.sessionId === sessionId);
      if (!exists) {
        draft.quickChat.sessions.push({ sessionId, workspaceId });
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
