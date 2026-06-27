import type { StateCreator } from "zustand";
import {
  getStoredCollapsedSubtaskParents,
  getStoredOrderedTaskIds,
  getStoredPinnedTaskIds,
  getStoredSidebarActiveViewId,
  getStoredSidebarDraft,
  getStoredSidebarUserViews,
  getStoredSubtaskOrderByParentId,
  setLocalStorage,
  setStoredCollapsedSubtaskParents,
  setStoredQuickChatName,
  setStoredSidebarUserViews,
} from "@/lib/local-storage";
import { buildDismissedAgentErrors } from "./dismissed-agent-errors-actions";
import {
  DEFAULT_SECTION_EXPANDED,
  buildAppSidebarActions,
  loadAppSidebarState,
} from "./app-sidebar-actions";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "@/components/app-sidebar/app-sidebar-constants";
import { buildSidebarTaskPrefsActions } from "./sidebar-task-prefs-actions";
import { buildSidebarViewActions } from "./sidebar-view-actions";
import { DEFAULT_ACTIVE_VIEW_ID, DEFAULT_VIEW } from "./sidebar-view-builtins";
import type { SidebarView, SidebarViewDraft, SortSpec } from "./sidebar-view-types";
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

export const KNOWN_SORT_KEYS = new Set<string>([
  "state",
  "updatedAt",
  "createdAt",
  "title",
  "custom",
]);

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
  connection: { status: "disconnected", error: null },
  mobileKanban: { activeColumnIndex: 0, isMenuOpen: false, isSearchOpen: false },
  mobileSession: { activePanelBySessionId: {}, isTaskSwitcherOpen: false },
  chatInput: { planModeBySessionId: {} },
  documentPanel: { activeDocumentBySessionId: {} },
  quickChat: { isOpen: false, sessions: [], activeSessionId: null },
  configChat: { isOpen: false, sessions: [], activeSessionId: null, workspaceId: null },
  sessionFailureNotification: null,
  bottomTerminal: { isOpen: false, pendingCommand: null },
  sidebarViews: loadSidebarState(),
  collapsedSubtaskParents: [],
  kanbanPreviewedTaskId: null,
  sidebarTaskPrefs: { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} },
  appSidebar: {
    collapsed: false,
    sectionExpanded: { ...DEFAULT_SECTION_EXPANDED },
    width: APP_SIDEBAR_EXPANDED_WIDTH,
    settingsMode: false,
  },
  dismissedAgentErrors: {},
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
    setMobileKanbanSearchOpen: (open: boolean) =>
      set((draft) => {
        draft.mobileKanban.isSearchOpen = open;
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
  sidebarTaskPrefs: {
    pinnedTaskIds: getStoredPinnedTaskIds(),
    orderedTaskIds: getStoredOrderedTaskIds(),
    subtaskOrderByParentId: getStoredSubtaskOrderByParentId(),
  },
  appSidebar: loadAppSidebarState(),
  ...buildAppSidebarActions(set),
  ...buildPreviewActions(set),
  ...buildMobileActions(set),
  ...buildBottomTerminalActions(set),
  ...buildConfigChatActions(set),
  ...buildSidebarViewActions(set, get),
  ...buildSidebarTaskPrefsActions(set, get),
  ...buildCollapsedSubtaskActions(set, get),
  ...buildDismissedAgentErrors(set),
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
    }),
  setActiveDocument: (sessionId, doc) =>
    set((draft) => {
      draft.documentPanel.activeDocumentBySessionId[sessionId] = doc;
      setLocalStorage(`active-document-${sessionId}`, doc as ActiveDocument | null);
    }),
  setKanbanPreviewedTaskId: (taskId) =>
    set((draft) => {
      if (draft.kanbanPreviewedTaskId === taskId) return;
      draft.kanbanPreviewedTaskId = taskId;
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
  renameQuickChatSession: (sessionId, name) => {
    let renamed = false;
    set((draft) => {
      const session = draft.quickChat.sessions.find((s) => s.sessionId === sessionId);
      if (session) {
        session.name = name;
        renamed = true;
      }
    });
    if (renamed) setStoredQuickChatName(sessionId, name);
  },
  setSessionFailureNotification: (n) =>
    set((draft) => {
      draft.sessionFailureNotification = n;
    }),
});
