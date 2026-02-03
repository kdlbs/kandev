import type { StateCreator } from 'zustand';
import { setLocalStorage } from '@/lib/local-storage';
import type { UISlice, UISliceState } from './types';

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
  connection: { status: 'disconnected', error: null },
  mobileKanban: { activeColumnIndex: 0, isMenuOpen: false },
  chatInput: { planModeBySessionId: {} },
};

export const createUISlice: StateCreator<UISlice, [['zustand/immer', never]], [], UISlice> = (
  set,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  _get
) => ({
  ...defaultUIState,
  setPreviewOpen: (sessionId, open) =>
    set((draft) => {
      draft.previewPanel.openBySessionId[sessionId] = open;
      setLocalStorage(`preview-open-${sessionId}`, open);
    }),
  togglePreviewOpen: (sessionId) =>
    set((draft) => {
      const current = draft.previewPanel.openBySessionId[sessionId] ?? false;
      draft.previewPanel.openBySessionId[sessionId] = !current;
      setLocalStorage(`preview-open-${sessionId}`, !current);
    }),
  setPreviewView: (sessionId, view) =>
    set((draft) => {
      draft.previewPanel.viewBySessionId[sessionId] = view;
      setLocalStorage(`preview-view-${sessionId}`, view);
    }),
  setPreviewDevice: (sessionId, device) =>
    set((draft) => {
      draft.previewPanel.deviceBySessionId[sessionId] = device;
      setLocalStorage(`preview-device-${sessionId}`, device);
    }),
  setPreviewStage: (sessionId, stage) =>
    set((draft) => {
      draft.previewPanel.stageBySessionId[sessionId] = stage;
    }),
  setPreviewUrl: (sessionId, url) =>
    set((draft) => {
      draft.previewPanel.urlBySessionId[sessionId] = url;
    }),
  setPreviewUrlDraft: (sessionId, url) =>
    set((draft) => {
      draft.previewPanel.urlDraftBySessionId[sessionId] = url;
    }),
  setRightPanelActiveTab: (sessionId, tab) =>
    set((draft) => {
      draft.rightPanel.activeTabBySessionId[sessionId] = tab;
    }),
  setConnectionStatus: (status, error) =>
    set((draft) => {
      draft.connection.status = status;
      draft.connection.error = error ?? null;
    }),
  setMobileKanbanColumnIndex: (index) =>
    set((draft) => {
      draft.mobileKanban.activeColumnIndex = index;
    }),
  setMobileKanbanMenuOpen: (open) =>
    set((draft) => {
      draft.mobileKanban.isMenuOpen = open;
    }),
  setPlanMode: (sessionId, enabled) =>
    set((draft) => {
      draft.chatInput.planModeBySessionId[sessionId] = enabled;
      setLocalStorage(`plan-mode-${sessionId}`, enabled);
    }),
});
