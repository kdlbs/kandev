import type { ConnectionStatus } from "@/lib/types/connection";
import type {
  FilterClause,
  GroupKey,
  SidebarSliceState,
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

export type ConnectionState = {
  status: ConnectionStatus;
  error: string | null;
};

export type MobileKanbanState = {
  activeColumnIndex: number;
  isMenuOpen: boolean;
  isSearchOpen: boolean;
};

export type MobileSessionPanel = "chat" | "plan" | "changes" | "files" | "terminal";

export type MobileSessionState = {
  activePanelBySessionId: Record<string, MobileSessionPanel>;
  isTaskSwitcherOpen: boolean;
};

export type ChatInputState = {
  planModeBySessionId: Record<string, boolean>;
};

export type ActiveDocument =
  | { type: "plan"; taskId: string }
  | { type: "file"; path: string; name: string };

export type DocumentPanelState = {
  activeDocumentBySessionId: Record<string, ActiveDocument | null>;
};

export type QuickChatSessionKind = "chat" | "config";

export type QuickChatSession = {
  kind: QuickChatSessionKind;
  sessionId: string;
  workspaceId: string;
  name?: string;
  agentProfileId?: string;
  initialPrompt?: string;
};

export type QuickChatState = {
  isOpen: boolean;
  sessions: QuickChatSession[];
  activeSessionId: string | null;
};

export type SessionFailureNotification = {
  sessionId: string;
  taskId: string;
  message: string;
};

export type TaskDeletedNotification = {
  taskId: string;
  /** Task title, when known, so the toast can name it. */
  title?: string;
  /** Backend deletion reason (e.g. "pr_approved_by_user"), when known. */
  reason?: string;
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
};

export type UISliceState = {
  previewPanel: PreviewPanelState;
  rightPanel: RightPanelState;
  connection: ConnectionState;
  mobileKanban: MobileKanbanState;
  mobileSession: MobileSessionState;
  chatInput: ChatInputState;
  documentPanel: DocumentPanelState;
  quickChat: QuickChatState;
  sessionFailureNotification: SessionFailureNotification | null;
  /** Set when the focused task is deleted live, so a toast can explain why. */
  taskDeletedNotification: TaskDeletedNotification | null;
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
  setMobileKanbanColumnIndex: (index: number) => void;
  setMobileKanbanMenuOpen: (open: boolean) => void;
  setMobileKanbanSearchOpen: (open: boolean) => void;
  setMobileSessionPanel: (sessionId: string, panel: MobileSessionPanel) => void;
  setMobileSessionTaskSwitcherOpen: (open: boolean) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
  setActiveDocument: (sessionId: string, doc: ActiveDocument | null) => void;
  openQuickChat: (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind?: QuickChatSessionKind,
  ) => void;
  addQuickChatSession: (
    sessionId: string,
    workspaceId: string,
    agentProfileId?: string,
    kind?: QuickChatSessionKind,
  ) => void;
  closeQuickChat: () => void;
  closeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string, workspaceId: string) => void;
  renameQuickChatSession: (sessionId: string, name: string) => void;
  setQuickChatInitialPrompt: (sessionId: string, prompt?: string) => void;
  setSessionFailureNotification: (n: SessionFailureNotification | null) => void;
  setTaskDeletedNotification: (n: TaskDeletedNotification | null) => void;
  toggleBottomTerminal: () => void;
  openBottomTerminalWithCommand: (command: string) => void;
  clearBottomTerminalCommand: () => void;
  setSidebarActiveView: (viewId: string) => void;
  createSidebarView: () => string | null;
  updateSidebarDraft: (
    patch: Partial<{ filters: FilterClause[]; sort: SortSpec; group: GroupKey }>,
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
  /** Record multiple sidebar badge acknowledgements with one localStorage merge. */
  acknowledgeAgentErrors: (stamps: Record<string, string>) => void;
  /** Record that `stamp` has been dismissed for `sessionId`. */
  dismissAgentError: (sessionId: string, stamp: string) => void;
};

export type { SidebarView, SidebarViewDraft };

export type UISlice = UISliceState & UISliceActions;
