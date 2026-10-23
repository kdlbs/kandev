import type { FileEditorState } from "./dockview-store";

export type FileDraftState = {
  openFiles: Map<string, FileEditorState>;
  fileDrafts: Map<string, Map<string, FileEditorState>>;
  activeFileSessionId: string | null;
  fileSessionEpoch: symbol;
  mobileFileKeys: Map<string, string>;
};

type StoreSet = (
  update: Partial<FileDraftState> | ((state: FileDraftState) => Partial<FileDraftState>),
) => void;

function publishRecord(state: FileDraftState, key: string, file: FileEditorState) {
  const openFiles = new Map(state.openFiles).set(key, file);
  const files = new Map(state.fileDrafts.get(state.activeFileSessionId ?? ""));
  files.set(key, file);
  return {
    openFiles,
    fileDrafts: new Map(state.fileDrafts).set(state.activeFileSessionId ?? "", files),
  };
}

export function buildFileStateActions(set: StoreSet) {
  return {
    activateFileSession: (sessionId: string | null) => {
      set((state) => ({
        activeFileSessionId: sessionId,
        fileSessionEpoch:
          state.activeFileSessionId === sessionId ? state.fileSessionEpoch : Symbol(),
        openFiles: new Map(state.fileDrafts.get(sessionId ?? "")),
      }));
    },
    selectFileDraft: (key: string | null) => {
      set((state) => {
        const mobileFileKeys = new Map(state.mobileFileKeys);
        if (key === null) mobileFileKeys.delete(state.activeFileSessionId ?? "");
        else mobileFileKeys.set(state.activeFileSessionId ?? "", key);
        return { mobileFileKeys };
      });
    },
    setFileState: (key: string, file: FileEditorState) => {
      set((state) => publishRecord(state, key, { ...file, instanceId: Symbol() }));
    },
    ensureFileState: (key: string, file: FileEditorState) => {
      set((state) => {
        const retained = state.fileDrafts.get(state.activeFileSessionId ?? "")?.get(key);
        return publishRecord(state, key, retained ?? { ...file, instanceId: Symbol() });
      });
    },
    updateFileState: (key: string, updates: Partial<FileEditorState>) => {
      set((state) => {
        const file = state.openFiles.get(key);
        return file
          ? publishRecord(state, key, { ...file, ...updates, instanceId: file.instanceId })
          : {};
      });
    },
    removeFileState: (key: string) => {
      set((state) => {
        const openFiles = new Map(state.openFiles);
        openFiles.delete(key);
        const files = new Map(state.fileDrafts.get(state.activeFileSessionId ?? ""));
        files.delete(key);
        return {
          openFiles,
          fileDrafts: new Map(state.fileDrafts).set(state.activeFileSessionId ?? "", files),
        };
      });
    },
    clearFileStates: () => set({ openFiles: new Map() }),
  };
}
