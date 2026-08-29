import type { ConnectionIssueSeverity, ConnectionStatus } from "@/lib/types/connection";
import type { HealthCheckSummary, HealthIssue, SystemHealthResponse } from "@/lib/types/health";
import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import type {
  FilterClause,
  GroupKey,
  SidebarSliceState,
  SidebarTaskRowPresentation,
  SidebarView,
  SidebarViewDraft,
  SortSpec,
} from "./sidebar-view-types";

export type PreviewStage = "closed" | "logs" | "preview";
export type PreviewViewMode = "preview" | "output";
export type PreviewDevicePreset = "desktop" | "tablet" | "mobile";

export type PreviewPanelState = {
  openBySessionId: Record<string, boolean>;
  viewBySessionId: Record<string, PreviewViewMode>;
  deviceBySessionId: Record<string, PreviewDevicePreset>;
  stageBySessionId: Record<string, PreviewStage>;
  urlBySessionId: Record<string, string>;
  urlDraftBySessionId: Record<string, string>;
};

export type RightPanelState = {
  activeTabBySessionId: Record<string, string>;
};

export type DiffState = {
  files: Array<{ path: string; status: "A" | "M" | "D"; plus: number; minus: number }>;
};

export type ConnectionState = {
  status: ConnectionStatus;
  error: string | null;
  issueSeverity: ConnectionIssueSeverity;
};

export type MobileKanbanState = {
  /** Last selected workflow step id, keyed by workflow id (phone board). */
  activeStepIdByWorkflowId: Record<string, string>;
  /**
   * The workflow the phone board is currently showing, published by
   * `SwimlaneContainer` so the menu drawer configures the same board the user
   * is looking at rather than deriving a second notion of focus. Null off the
   * phone kanban.
   */
  focusedWorkflowId: string | null;
  isMenuOpen: boolean;
  isSearchOpen: boolean;
};

/** Core, host-defined mobile panels. Kept as a named union (rather than
 *  inlined into MobileSessionPanel) so existing `=== "chat"`-style narrowing
 *  still works unchanged after MobileSessionPanel grew a plugin variant. */
export type MobileSessionCorePanel =
  | "chat"
  | "plan"
  | "changes"
  | "files"
  | "terminal"
  | "review"
  | "prompt-history";

/** A plugin task panel id on mobile, `plugin:<pluginId>:<panelKey>` — see
 *  lib/state/layout-manager/plugin-panels.ts's pluginPanelId. */
export type MobileSessionPluginPanel = `plugin:${string}:${string}`;

export type MobileSessionPanel = MobileSessionCorePanel | MobileSessionPluginPanel;

export type MobileSessionState = {
  activePanelBySessionId: Record<string, MobileSessionPanel>;
  reviewItemIdBySessionId: Record<string, string>;
  isTaskSwitcherOpen: boolean;
};

export type ChatInputState = {
  planModeBySessionId: Record<string, boolean>;
  /** True while a session's agent.cancel request is awaiting settlement. */
  cancellingBySessionId: Record<string, boolean>;
};

export type TranscriptAutoScrollState = {
  /** Per-session auto-scroll preference. Absent = enabled (the default). */
  enabledBySessionId: Record<string, boolean>;
  /** Last known scrollTop for the native renderer, captured continuously so
   *  a disabled session's position survives a dockview panel remount. */
  scrollTopBySessionId: Record<string, number>;
};

export type ReviewPRSelectionState = {
  selectedKeyByTaskId: Record<string, string>;
};

export type ActiveDocument =
  | { type: "plan"; taskId: string }
  | { type: "file"; path: string; name: string };

export type DocumentPanelState = {
  activeDocumentBySessionId: Record<string, ActiveDocument | null>;
};

export type SystemHealthState = {
  issues: HealthIssue[];
  checks: HealthCheckSummary[];
  healthy: boolean;
  loaded: boolean;
  loading: boolean;
};

export type QuickChatSessionKind = "chat" | "config";

export type QuickTerminalStatus = "connecting" | "running" | "exited" | "error";

export type QuickTerminalTab = {
  tabId: string;
  workspaceId: string;
  sessionId: string | null;
  sequence: number;
  status: QuickTerminalStatus;
  exitCode?: number;
  error?: string;
};

export type QuickTerminalUpdate = {
  sequence?: number;
  sessionId?: string | null;
  status?: QuickTerminalStatus;
  exitCode?: number | null;
  error?: string | null;
};

export type QuickChatSession = {
  kind: QuickChatSessionKind;
  sessionId: string;
  workspaceId: string;
  /** Backing ephemeral task. Absent only on unstarted "New chat" setup tabs. */
  taskId?: string;
  name?: string;
  agentProfileId?: string;
  initialPrompt?: string;
};

export type QuickChatActiveKind = "conversation" | "terminal";

export type QuickChatSessionOwnership = {
  taskId?: string;
  workspaceId: string;
};

export type QuickChatSessionTombstone = {
  workspaceId: string;
  tombstonedAt: string;
};

export type QuickChatState = {
  isOpen: boolean;
  sessions: QuickChatSession[];
  activeSessionId: string | null;
  terminalTabs: QuickTerminalTab[];
  activeKind: QuickChatActiveKind;
  activeTerminalTabId: string | null;
  lastTerminalTabIdByWorkspace: Record<string, string>;
  unseenIdleByWorkspace: Record<string, Record<string, true>>;
  lastSettledAtBySession: Record<string, string>;
  sessionOwnership: Record<string, QuickChatSessionOwnership>;
  syncRevisionByWorkspace: Record<string, number>;
  tombstonedSessions: Record<string, QuickChatSessionTombstone>;
  /** Optimistic mixed-tab order keyed by workspace until the save settles. */
  tabOrderByWorkspace: Record<string, string[]>;
  tabOrderSyncErrorByWorkspace: Record<string, string | null>;
  tabOrderSyncPendingByWorkspace: Record<string, boolean>;
};

export type SessionFailureNotification = {
  sessionId: string;
  taskId: string;
  message: string;
  /** Typed launch failures point users to the persistent task card. */
  isLaunchFailure?: boolean;
};

export type TaskDeletedNotification = {
  taskId: string;
  /** Task title, when known, so the toast can name it. */
  title?: string;
  /** Backend deletion reason (e.g. "pr_approved_by_user"), when known. */
  reason?: string;
};

export type UpdateAvailableNotification = {
  version: string;
  url?: string;
  title: string;
  body: string;
  occurrence_id: string;
};

export type BottomTerminalState = {
  isOpen: boolean;
  pendingCommand: string | null;
};

export type SidebarTaskPrefsState = {
  /** Pinned task IDs in display order (first ID renders highest within its group). */
  pinnedTaskIds: string[];
  /** Manual order. Tasks not present fall back to the active sort. */
  orderedTaskIds: string[];
  /**
   * Per-parent subtask order. Keyed by parent task id; value is the ordered
   * subtask ids. Subtasks not listed fall back to the active sort.
   * Independent of the global `orderedTaskIds` and the view's sort spec.
   */
  subtaskOrderByParentId: Record<string, string[]>;
  syncError?: string | null;
  syncPending?: boolean;
};

/** Settings menu shape + open branches, per device (localStorage). */
export type SettingsMenuState = {
  /**
   * What the menu renders right now: the saved mode, or the unsaved one the
   * Appearance page is previewing. Read this everywhere the menu is drawn.
   */
  mode: SettingsMenuMode;
  /** The persisted value. `restoreSettingsMenuMode` reverts `mode` to this. */
  savedMode: SettingsMenuMode;
  /**
   * Branch keys open in `persistent` mode. Accordion mode derives its single
   * open path from the route instead, so it never reads or writes this.
   */
  expandedKeys: string[];
};

/** Agent rich-output chart motion preference, per device (localStorage). */
export type RichOutputMotionState = {
  /** The value rendered now, including an unsaved Appearance preview. */
  enabled: boolean;
  /** The persisted value restored when the Appearance draft is discarded. */
  savedEnabled: boolean;
};

/** Unified AppSidebar collapse + per-section expand state (localStorage). */
export type AppSidebarState = {
  collapsed: boolean;
  /** Keyed by section id: "tasks", "projects", "agents", "settings". */
  sectionExpanded: Record<string, boolean>;
  /** User-resized expanded width in pixels. */
  width: number;
  /**
   * When true the whole sidebar is taken over by the settings tree (toggled by
   * the footer gear). Transient view mode — intentionally NOT persisted so a
   * reload never traps the user in settings.
   */
  settingsMode: boolean;
  /**
   * Shared Improve Kandev dialog open flag. Owned by the store so the footer
   * button and the New Task entry (inside the Improve Kandev workspace) open
   * the same dialog instance. Transient, never persisted.
   */
  improveDialogOpen: boolean;
  /**
   * Open state of the workspace picker in the expanded sidebar header. Owned by
   * the store so the global WORKSPACE_PICKER shortcut can open the menu without
   * reaching into the DOM. Transient, never persisted. Only the sidebar-header
   * picker instance binds to it — the mobile sheet keeps its own local state.
   */
  workspacePickerOpen: boolean;
};

export type UISliceState = {
  previewPanel: PreviewPanelState;
  rightPanel: RightPanelState;
  diffs: DiffState;
  connection: ConnectionState;
  mobileKanban: MobileKanbanState;
  mobileSession: MobileSessionState;
  chatInput: ChatInputState;
  transcriptAutoScroll: TranscriptAutoScrollState;
  reviewPRSelection: ReviewPRSelectionState;
  documentPanel: DocumentPanelState;
  systemHealth: SystemHealthState;
  quickChat: QuickChatState;
  sessionFailureNotification: SessionFailureNotification | null;
  /** Set when the focused task is deleted live, so a toast can explain why. */
  taskDeletedNotification: TaskDeletedNotification | null;
  /** Set when the background updates poller reports a newly detected release. */
  updateAvailableNotification: UpdateAvailableNotification | null;
  bottomTerminal: BottomTerminalState;
  sidebarViews: SidebarSliceState;
  /** Parent task IDs whose subtasks are collapsed in the sidebar. Tab-scoped (sessionStorage). */
  collapsedSubtaskParents: string[];
  /** Task ID currently shown in the kanban preview side-panel, or null if closed. */
  kanbanPreviewedTaskId: string | null;
  /** Sidebar pin + manual-order. Synced to backend, with localStorage fallback. */
  sidebarTaskPrefs: SidebarTaskPrefsState;
  /** Unified AppSidebar collapse + section expand state (localStorage). */
  appSidebar: AppSidebarState;
  /** Settings menu shape + open branches (localStorage). */
  settingsMenu: SettingsMenuState;
  /** Agent rich-output chart animation preference (localStorage). */
  richOutputMotion: RichOutputMotionState;
  /**
   * Most recently dismissed `last_agent_error` stamp per sessionId. Shared by
   * the chat banner and the sidebar error icon so dismissing the banner also
   * hides the icon. Persisted to localStorage.
   */
  dismissedAgentErrors: Record<string, string>;
  /**
   * Most recently acknowledged sidebar `last_agent_error` stamp per sessionId.
   * This suppresses task-row badges after the sidebar can prove an error is
   * stale, without hiding the chat banner.
   */
  acknowledgedAgentErrors: Record<string, string>;
};

export type UISliceActions = {
  setPreviewOpen: (sessionId: string, open: boolean) => void;
  togglePreviewOpen: (sessionId: string) => void;
  setPreviewView: (sessionId: string, view: PreviewViewMode) => void;
  setPreviewDevice: (sessionId: string, device: PreviewDevicePreset) => void;
  setPreviewStage: (sessionId: string, stage: PreviewStage) => void;
  setPreviewUrl: (sessionId: string, url: string) => void;
  setPreviewUrlDraft: (sessionId: string, url: string) => void;
  setRightPanelActiveTab: (sessionId: string, tab: string) => void;
  setConnectionStatus: (status: ConnectionState["status"], error?: string | null) => void;
  setConnectionIssueSeverity: (severity: ConnectionIssueSeverity) => void;
  setMobileKanbanActiveStep: (workflowId: string, stepId: string) => void;
  setMobileKanbanMenuOpen: (open: boolean) => void;
  setMobileKanbanSearchOpen: (open: boolean) => void;
  setMobileKanbanFocusedWorkflow: (workflowId: string | null) => void;
  setMobileSessionPanel: (sessionId: string, panel: MobileSessionPanel) => void;
  setMobileSessionReview: (sessionId: string, reviewItemId: string | null) => void;
  setMobileSessionTaskSwitcherOpen: (open: boolean) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
  setCancelTurnPending: (sessionId: string, pending: boolean) => void;
  setTranscriptAutoScrollEnabled: (sessionId: string, enabled: boolean) => void;
  setTranscriptScrollTop: (sessionId: string, scrollTop: number) => void;
  setReviewPRSelection: (taskId: string, selectedKey: string) => void;
  setActiveDocument: (sessionId: string, doc: ActiveDocument | null) => void;
  setSystemHealth: (response: SystemHealthResponse) => void;
  setSystemHealthLoading: (loading: boolean) => void;
  invalidateSystemHealth: () => void;
  openQuickChat: (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind?: QuickChatSessionKind,
    taskId?: string,
  ) => void;
  addQuickChatSession: (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind?: QuickChatSessionKind,
    taskId?: string,
  ) => void;
  reuseOrCreateQuickTerminal: (workspaceId: string) => string;
  createQuickTerminal: (workspaceId: string) => string;
  updateQuickTerminal: (tabId: string, update: QuickTerminalUpdate) => void;
  activateQuickTerminal: (tabId: string, workspaceId: string) => void;
  removeQuickTerminal: (tabId: string) => void;
  closeQuickChat: () => void;
  closeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string, workspaceId: string) => void;
  renameQuickChatSession: (sessionId: string, name: string) => void;
  /** Replaces a workspace's quick-chat tabs with the server's authoritative list. */
  syncQuickChatSessions: (workspaceId: string, sessions: QuickChatSession[]) => void;
  /** Replaces a workspace's terminal descriptors with the server's list. */
  syncQuickTerminalTabs: (workspaceId: string, tabs: QuickTerminalTab[]) => void;
  /** Adds or updates a tab observed on the wire, without stealing focus. */
  upsertQuickChatSessionFromEvent: (session: QuickChatSession) => void;
  /** Drops tabs whose backing task was deleted (possibly on another device). */
  removeQuickChatSessionsForTask: (taskId: string) => void;
  markQuickChatUnseenIdle: (sessionId: string, workspaceId: string) => void;
  clearQuickChatUnseenIdle: (sessionId?: string, workspaceId?: string) => void;
  /** Records a settle generation and returns whether it was not previously observed. */
  recordQuickChatSettled: (sessionId: string, updatedAt: string) => boolean;
  /** Removes a server-backed quick-chat session and suppresses late task events. */
  removeQuickChatSession: (sessionId: string) => void;
  /** Sets the optimistic mixed conversation/terminal order for a workspace. */
  setQuickChatTabOrder: (workspaceId: string, order: string[]) => void;
  /** Clears a matching optimistic order after its authoritative save succeeds. */
  clearQuickChatTabOrder: (workspaceId: string, expectedOrder: string[]) => void;
  /** Updates the pending/error state for a workspace order save. */
  setQuickChatTabOrderSyncState: (
    workspaceId: string,
    state: { pending: boolean; error: string | null },
  ) => void;
  setQuickChatInitialPrompt: (sessionId: string, prompt?: string) => void;
  setSessionFailureNotification: (n: SessionFailureNotification | null) => void;
  setTaskDeletedNotification: (n: TaskDeletedNotification | null) => void;
  setUpdateAvailableNotification: (n: UpdateAvailableNotification | null) => void;
  toggleBottomTerminal: () => void;
  openBottomTerminalWithCommand: (command: string) => void;
  clearBottomTerminalCommand: () => void;
  setSidebarActiveView: (viewId: string) => void;
  createSidebarView: () => string | null;
  updateSidebarDraft: (
    patch: Partial<{
      filters: FilterClause[];
      sort: SortSpec;
      group: GroupKey;
      taskRow: SidebarTaskRowPresentation;
    }>,
  ) => void;
  saveSidebarDraftAs: (name: string) => void;
  saveSidebarDraftOverwrite: () => void;
  discardSidebarDraft: () => void;
  deleteSidebarView: (viewId: string) => void;
  renameSidebarView: (viewId: string, name: string) => void;
  duplicateSidebarView: (viewId: string, name: string) => void;
  reorderSidebarViews: (activeViewId: string, overViewId: string) => void;
  toggleSidebarGroupCollapsed: (viewId: string, groupKey: string) => void;
  toggleSubtaskCollapsed: (parentTaskId: string) => void;
  clearSidebarSyncError: () => void;
  clearSidebarTaskPrefsSyncError: () => void;
  setKanbanPreviewedTaskId: (taskId: string | null) => void;
  togglePinnedTask: (taskId: string) => void;
  /** Pin every id that isn't already pinned (bulk "Pin" for multi-select). */
  pinTasks: (taskIds: string[]) => void;
  /** Unpin every id that is currently pinned (bulk "Unpin" for multi-select). */
  unpinTasks: (taskIds: string[]) => void;
  setSidebarTaskOrder: (orderedTaskIds: string[]) => void;
  /** Replace the stored subtask order for a parent task. Empty array clears it. */
  setSubtaskOrder: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  /**
   * Drop a task ID from pinned, ordered, and subtask-order arrays in-memory
   * and persist. Called on task deletion so the in-memory state doesn't
   * out-of-date the already-cleaned localStorage and silently re-write the
   * deleted ID back.
   */
  removeTaskFromSidebarPrefs: (taskId: string) => void;
  toggleAppSidebar: () => void;
  setAppSidebarCollapsed: (collapsed: boolean) => void;
  toggleAppSidebarSection: (sectionId: string, defaultExpanded?: boolean) => void;
  setAppSidebarWidth: (width: number) => void;
  setAppSidebarSettingsMode: (settingsMode: boolean) => void;
  toggleAppSidebarSettingsMode: () => void;
  /** Open/close the shared Improve Kandev dialog (footer + New Task routing). */
  setImproveDialogOpen: (open: boolean) => void;
  /**
   * Open/close the sidebar-header workspace picker. Opening force-expands the
   * sidebar, since the trigger renders only in the expanded header.
   */
  setWorkspacePickerOpen: (open: boolean) => void;
  /** Render `mode` without persisting it — the Appearance page's live preview. */
  previewSettingsMenuMode: (mode: SettingsMenuMode) => void;
  /** Persist `mode` for this device. Called by the settings save coordinator. */
  commitSettingsMenuMode: (mode: SettingsMenuMode) => void;
  /** Drop an unsaved preview and render the persisted mode again. */
  restoreSettingsMenuMode: () => void;
  setSettingsMenuExpandedKeys: (keys: string[]) => void;
  /** Preview rich-output chart motion without persisting it. */
  previewRichOutputAnimations: (enabled: boolean) => void;
  /** Persist rich-output chart motion for this device. */
  commitRichOutputAnimations: (enabled: boolean) => void;
  /** Restore the persisted rich-output chart motion preference. */
  restoreRichOutputAnimations: () => void;
  /** Record multiple sidebar badge acknowledgements with one localStorage merge. */
  acknowledgeAgentErrors: (stamps: Record<string, string>) => void;
  /** Record that `stamp` has been dismissed for `sessionId`. */
  dismissAgentError: (sessionId: string, stamp: string) => void;
};

export type { SidebarView, SidebarViewDraft };

export type UISlice = UISliceState & UISliceActions;
