export type PreviewStage = 'closed' | 'logs' | 'preview';
export type PreviewViewMode = 'preview' | 'output';
export type PreviewDevicePreset = 'desktop' | 'tablet' | 'mobile';

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
  files: Array<{ path: string; status: 'A' | 'M' | 'D'; plus: number; minus: number }>;
};

export type ConnectionState = {
  status: 'disconnected' | 'connecting' | 'connected' | 'error' | 'reconnecting';
  error: string | null;
};

export type MobileKanbanState = {
  activeColumnIndex: number;
  isMenuOpen: boolean;
};

export type MobileSessionPanel = 'chat' | 'plan' | 'changes' | 'files' | 'terminal';

export type MobileSessionState = {
  activePanelBySessionId: Record<string, MobileSessionPanel>;
  isTaskSwitcherOpen: boolean;
};

export type ChatInputState = {
  planModeBySessionId: Record<string, boolean>;
};

export type UISliceState = {
  previewPanel: PreviewPanelState;
  rightPanel: RightPanelState;
  diffs: DiffState;
  connection: ConnectionState;
  mobileKanban: MobileKanbanState;
  mobileSession: MobileSessionState;
  chatInput: ChatInputState;
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
  setConnectionStatus: (status: ConnectionState['status'], error?: string | null) => void;
  setMobileKanbanColumnIndex: (index: number) => void;
  setMobileKanbanMenuOpen: (open: boolean) => void;
  setMobileSessionPanel: (sessionId: string, panel: MobileSessionPanel) => void;
  setMobileSessionTaskSwitcherOpen: (open: boolean) => void;
  setPlanMode: (sessionId: string, enabled: boolean) => void;
};

export type UISlice = UISliceState & UISliceActions;
