import type { StateCreator } from "zustand";
import {
  getStoredCollapsedSubtaskParents,
  setLocalStorage,
  setStoredCollapsedSubtaskParents,
  setStoredAutoScrollEnabled,
  setStoredAutoScrollTop,
} from "@/lib/local-storage";
import { buildDismissedAgentErrors } from "./dismissed-agent-errors-actions";
import {
  DEFAULT_SECTION_EXPANDED,
  buildAppSidebarActions,
  loadAppSidebarState,
} from "./app-sidebar-actions";
import { buildSettingsMenuActions, loadSettingsMenuState } from "./settings-menu-actions";
import {
  buildRichOutputMotionActions,
  loadRichOutputMotionState,
} from "./rich-output-motion-actions";
import { DEFAULT_SETTINGS_MENU_MODE } from "@/lib/settings/settings-menu-mode";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "@/components/app-sidebar/app-sidebar-constants";
import { buildSidebarTaskPrefsActions } from "./sidebar-task-prefs-actions";
import { buildSidebarViewActions } from "./sidebar-view-actions";
import { DEFAULT_VIEW } from "./sidebar-view-builtins";
import type { SidebarView, SidebarViewDraft, SortSpec } from "./sidebar-view-types";
import { cloneSidebarTaskRowPresentation } from "./sidebar-task-row-presentation";
import type { SystemHealthResponse } from "@/lib/types/health";
import type { ActiveDocument, UISlice, UISliceState } from "./types";
import { buildQuickChatActions } from "./quick-chat-actions";
import { buildQuickTerminalActions } from "./quick-terminal-actions";

/** Default sidebar view state: the single built-in "All tasks" view, active, no draft. */
function createDefaultSidebarState(): UISliceState["sidebarViews"] {
  return { views: [DEFAULT_VIEW], activeViewId: DEFAULT_VIEW.id, draft: null, syncError: null };
}

export const KNOWN_DIMENSIONS = new Set<string>([
  "archived",
  "state",
  "workflow",
  "workflowStep",
  "executorType",
  "repository",
  "hasDiff",
  "hasPR",
  "isPRReview",
  "isIssueWatch",
  "titleMatch",
]);

export const KNOWN_SORT_KEYS = new Set<string>([
  "state",
  "updatedAt",
  "lastActivityAt",
  "createdAt",
  "title",
  "custom",
]);

/**
 * Drops clauses whose dimension is no longer known (e.g. renamed or removed
 * in an upgrade), and resets stale sort keys, so the popover does not crash
 * when rendering stored views.
 */
export function migrateView(view: SidebarView): SidebarView {
  const sort: SortSpec = KNOWN_SORT_KEYS.has(view.sort.key)
    ? view.sort
    : { key: "state", direction: view.sort.direction };
  return {
    ...view,
    filters: view.filters.filter((c) => KNOWN_DIMENSIONS.has(c.dimension)),
    sort,
    taskRow: cloneSidebarTaskRowPresentation(view.taskRow),
  };
}

/** Drops removed filter dimensions from an in-flight saved-view draft. */
export function migrateSidebarViewDraft(draft: SidebarViewDraft): SidebarViewDraft {
  const sort: SortSpec = KNOWN_SORT_KEYS.has(draft.sort.key)
    ? draft.sort
    : { key: "state", direction: draft.sort.direction };
  return {
    ...draft,
    filters: draft.filters.filter((c) => KNOWN_DIMENSIONS.has(c.dimension)),
    sort,
    taskRow: cloneSidebarTaskRowPresentation(draft.taskRow),
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
  diffs: { files: [] },
  connection: { status: "disconnected", error: null, issueSeverity: "none" },
  mobileKanban: {
    activeStepIdByWorkflowId: {},
    isMenuOpen: false,
    isSearchOpen: false,
    focusedWorkflowId: null,
  },
  mobileSession: {
    activePanelBySessionId: {},
    reviewItemIdBySessionId: {},
    isTaskSwitcherOpen: false,
  },
  chatInput: { planModeBySessionId: {}, cancellingBySessionId: {} },
  transcriptAutoScroll: {
    enabledBySessionId: {},
    scrollTopBySessionId: {},
  },
  reviewPRSelection: { selectedKeyByTaskId: {} },
  documentPanel: { activeDocumentBySessionId: {} },
  systemHealth: { issues: [], checks: [], healthy: true, loaded: false, loading: false },
  quickChat: {
    isOpen: false,
    sessions: [],
    activeSessionId: null,
    terminalTabs: [],
    activeKind: "conversation",
    activeTerminalTabId: null,
    lastTerminalTabIdByWorkspace: {},
    unseenIdleByWorkspace: {},
    lastSettledAtBySession: {},
    sessionOwnership: {},
    syncRevisionByWorkspace: {},
    tombstonedSessions: {},
  },
  sessionFailureNotification: null,
  taskDeletedNotification: null,
  updateAvailableNotification: null,
  bottomTerminal: { isOpen: false, pendingCommand: null },
  sidebarViews: createDefaultSidebarState(),
  collapsedSubtaskParents: [],
  kanbanPreviewedTaskId: null,
  sidebarTaskPrefs: { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} },
  appSidebar: {
    collapsed: false,
    sectionExpanded: { ...DEFAULT_SECTION_EXPANDED },
    width: APP_SIDEBAR_EXPANDED_WIDTH,
    settingsMode: false,
    improveDialogOpen: false,
    workspacePickerOpen: false,
  },
  settingsMenu: {
    mode: DEFAULT_SETTINGS_MENU_MODE,
    savedMode: DEFAULT_SETTINGS_MENU_MODE,
    expandedKeys: [],
  },
  richOutputMotion: {
    enabled: true,
    savedEnabled: true,
  },
  acknowledgedAgentErrors: {},
  dismissedAgentErrors: {},
};

type ImmerSet = Parameters<typeof createUISlice>[0];

/** Builds the preview-panel actions (open/toggle/view/device/stage/url), each persisting to localStorage where noted. */
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

/** Builds the mobile Kanban view's column-index, menu, and search-open actions. */
function buildMobileActions(set: ImmerSet) {
  return {
    setMobileKanbanActiveStep: (workflowId: string, stepId: string) =>
      set((draft) => {
        draft.mobileKanban.activeStepIdByWorkflowId[workflowId] = stepId;
      }),
    setMobileKanbanMenuOpen: (open: boolean) =>
      set((draft) => {
        draft.mobileKanban.isMenuOpen = open;
      }),
    setMobileKanbanSearchOpen: (open: boolean) =>
      set((draft) => {
        draft.mobileKanban.isSearchOpen = open;
      }),
    setMobileKanbanFocusedWorkflow: (workflowId: string | null) =>
      set((draft) => {
        draft.mobileKanban.focusedWorkflowId = workflowId;
      }),
    setMobileSessionPanel: (
      sessionId: string,
      panel: UISliceState["mobileSession"]["activePanelBySessionId"][string],
    ) =>
      set((draft) => {
        draft.mobileSession.activePanelBySessionId[sessionId] = panel;
      }),
    setMobileSessionReview: (sessionId: string, reviewItemId: string | null) =>
      set((draft) => {
        if (reviewItemId) {
          draft.mobileSession.reviewItemIdBySessionId[sessionId] = reviewItemId;
          draft.mobileSession.activePanelBySessionId[sessionId] = "review";
          return;
        }
        delete draft.mobileSession.reviewItemIdBySessionId[sessionId];
        if (draft.mobileSession.activePanelBySessionId[sessionId] === "review") {
          draft.mobileSession.activePanelBySessionId[sessionId] = "chat";
        }
      }),
    setMobileSessionTaskSwitcherOpen: (open: boolean) =>
      set((draft) => {
        draft.mobileSession.isTaskSwitcherOpen = open;
      }),
  };
}

/** Builds the bottom terminal's open/toggle and pending-command actions, persisting open state to localStorage. */
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

/** Builds the system-health cache's set/loading/invalidate actions. */
function buildSystemHealthActions(set: ImmerSet) {
  return {
    setSystemHealth: (response: SystemHealthResponse) =>
      set((draft) => {
        draft.systemHealth.issues = response.issues;
        draft.systemHealth.checks = response.checks ?? [];
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

/**
 * Builds the collapsed-subtask-parents toggle action. Tab-scoped collapse of
 * a parent task's subtasks, persisted via sessionStorage (survives reload /
 * task switch within the tab, resets on tab close). Not per-view and not
 * synced to the backend — purely visual.
 */
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

/** Builds the setters for session-failure, task-deleted, and update-available notifications. */
function buildNotificationActions(set: ImmerSet) {
  return {
    setSessionFailureNotification: (n: UISlice["sessionFailureNotification"]) =>
      set((draft) => {
        draft.sessionFailureNotification = n;
      }),
    setTaskDeletedNotification: (n: UISlice["taskDeletedNotification"]) =>
      set((draft) => {
        draft.taskDeletedNotification = n;
      }),
    setUpdateAvailableNotification: (n: UISlice["updateAvailableNotification"]) =>
      set((draft) => {
        draft.updateAvailableNotification = n;
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
  sidebarTaskPrefs: { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} },
  appSidebar: loadAppSidebarState(),
  settingsMenu: loadSettingsMenuState(),
  richOutputMotion: loadRichOutputMotionState(),
  ...buildAppSidebarActions(set),
  ...buildSettingsMenuActions(set),
  ...buildRichOutputMotionActions(set),
  ...buildPreviewActions(set),
  ...buildMobileActions(set),
  ...buildBottomTerminalActions(set),
  ...buildSidebarViewActions(set, get),
  ...buildSidebarTaskPrefsActions(set, get),
  ...buildCollapsedSubtaskActions(set, get),
  ...buildSystemHealthActions(set),
  ...buildDismissedAgentErrors(set),
  ...buildNotificationActions(set),
  ...buildQuickTerminalActions(set),
  ...buildQuickChatActions(set),
  setRightPanelActiveTab: (sessionId, tab) =>
    set((draft) => {
      draft.rightPanel.activeTabBySessionId[sessionId] = tab;
    }),
  setConnectionStatus: (status, error) =>
    set((draft) => {
      draft.connection.status = status;
      draft.connection.error = error ?? null;
    }),
  setConnectionIssueSeverity: (severity) =>
    set((draft) => {
      draft.connection.issueSeverity = severity;
    }),
  setPlanMode: (sessionId, enabled) =>
    set((draft) => {
      draft.chatInput.planModeBySessionId[sessionId] = enabled;
    }),
  setCancelTurnPending: (sessionId, pending) =>
    set((draft) => {
      if (pending) {
        draft.chatInput.cancellingBySessionId[sessionId] = true;
      } else {
        delete draft.chatInput.cancellingBySessionId[sessionId];
      }
    }),
  setTranscriptAutoScrollEnabled: (sessionId, enabled) =>
    set((draft) => {
      draft.transcriptAutoScroll.enabledBySessionId[sessionId] = enabled;
      setStoredAutoScrollEnabled(sessionId, enabled);
    }),
  setTranscriptScrollTop: (sessionId, scrollTop) =>
    set((draft) => {
      draft.transcriptAutoScroll.scrollTopBySessionId[sessionId] = scrollTop;
      setStoredAutoScrollTop(sessionId, scrollTop);
    }),
  setReviewPRSelection: (taskId, selectedKey) =>
    set((draft) => {
      draft.reviewPRSelection.selectedKeyByTaskId[taskId] = selectedKey;
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
});
