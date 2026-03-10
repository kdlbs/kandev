import type { StateCreator } from "zustand";
import { setLocalStorage } from "@/lib/local-storage";
import type { ActiveDocument, UISlice, UISliceState } from "./types";

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
  sessionFailureNotification: null,
  bottomTerminal: { isOpen: false },
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

export const createUISlice: StateCreator<UISlice, [["zustand/immer", never]], [], UISlice> = (
  set,
) => ({
  ...defaultUIState,
  ...buildPreviewActions(set),
  ...buildMobileActions(set),
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
  toggleBottomTerminal: () =>
    set((draft) => {
      const newValue = !draft.bottomTerminal.isOpen;
      draft.bottomTerminal.isOpen = newValue;
      // Persist to localStorage
      if (typeof window !== "undefined") {
        localStorage.setItem("bottom-terminal-open", String(newValue));
      }
    }),
});
