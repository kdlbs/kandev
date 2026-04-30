import type { HealthIssue, SystemHealthResponse } from "@/lib/types/health";
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

export type DiffState = {
  files: Array<{ path: string; status: "A" | "M" | "D"; plus: number; minus: number }>;
};

export type ConnectionState = {
  status: "disconnected" | "connecting" | "connected" | "error" | "reconnecting";
  error: string | null;
};

export type MobileKanbanState = {
  activeColumnIndex: number;
  isMenuOpen: boolean;
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

export type SystemHealthState = {
  issues: HealthIssue[];
  healthy: boolean;
  loaded: boolean;
  loading: boolean;
};

export type QuickChatState = {
  isOpen: boolean;
  sessions: Array<{
    sessionId: string;
    workspaceId: string;
    name?: string;
    agentProfileId?: string;
  }>;
  activeSessionId: string | null;
};

export type ConfigChatSession = {
  sessionId: string;
  workspaceId: string;
  name?: string;
};

export type ConfigChatState = {
  isOpen: boolean;
  sessions: ConfigChatSession[];
  activeSessionId: string | null;
  workspaceId: string | null;
};

export type SessionFailureNotification = {
  sessionId: string;
  taskId: string;
  message: string;
};

export type BottomTerminalState = {
  isOpen: boolean;
  pendingCommand: string | null;
};

export type UISliceState = {
  previewPanel: PreviewPanelState;
  rightPanel: RightPanelState;
  diffs: DiffState;
  connection: ConnectionState;
  mobileKanban: MobileKanbanState;
  mobileSession: MobileSessionState;
  chatInput: ChatInputState;
  documentPanel: DocumentPanelState;
  systemHealth: SystemHealthState;
  quickChat: QuickChatState;
  configChat: ConfigChatState;
  sessionFailureNotification: SessionFailureNotification | null;
  bottomTerminal: BottomTerminalState;
  sidebarViews: SidebarSliceState;
  /** Parent task IDs whose subtasks are collapsed in the sidebar. Tab-scoped (sessionStorage). */
  collapsedSubtaskParents: string[];
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
  setMobileSessionPanel: (sessionId: string, panel: MobileSessionPanel) => void;
  setMobileSessionTaskSwitcherOpen: (open: boolean) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
  setActiveDocument: (sessionId: string, doc: ActiveDocument | null) => void;
  setSystemHealth: (response: SystemHealthResponse) => void;
  setSystemHealthLoading: (loading: boolean) => void;
  invalidateSystemHealth: () => void;
  openQuickChat: (sessionId: string, workspaceId: string, agentProfileId?: string) => void;
  closeQuickChat: () => void;
  closeQuickChatSession: (sessionId: string) => void;
  setActiveQuickChatSession: (sessionId: string) => void;
  renameQuickChatSession: (sessionId: string, name: string) => void;
  openConfigChat: (sessionId: string, workspaceId: string) => void;
  startNewConfigChat: (workspaceId: string) => void;
  closeConfigChat: () => void;
  closeConfigChatSession: (sessionId: string) => void;
  setActiveConfigChatSession: (sessionId: string) => void;
  renameConfigChatSession: (sessionId: string, name: string) => void;
  setSessionFailureNotification: (n: SessionFailureNotification | null) => void;
  toggleBottomTerminal: () => void;
  openBottomTerminalWithCommand: (command: string) => void;
  clearBottomTerminalCommand: () => void;
  setSidebarActiveView: (viewId: string) => void;
  updateSidebarDraft: (
    patch: Partial<{ filters: FilterClause[]; sort: SortSpec; group: GroupKey }>,
  ) => void;
  saveSidebarDraftAs: (name: string) => void;
  saveSidebarDraftOverwrite: () => void;
  discardSidebarDraft: () => void;
  deleteSidebarView: (viewId: string) => void;
  renameSidebarView: (viewId: string, name: string) => void;
  duplicateSidebarView: (viewId: string, name: string) => void;
  toggleSidebarGroupCollapsed: (viewId: string, groupKey: string) => void;
  toggleSubtaskCollapsed: (parentTaskId: string) => void;
  clearSidebarSyncError: () => void;
  migrateLocalViewsToBackend: () => void;
};

export type { SidebarView, SidebarViewDraft };

export type UISlice = UISliceState & UISliceActions;
