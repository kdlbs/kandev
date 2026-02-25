import type { FileEditorState } from "./dockview-store";

type StoreSet = (
  partial:
    | { openFiles: Map<string, FileEditorState> }
    | ((s: { openFiles: Map<string, FileEditorState> }) => { openFiles: Map<string, FileEditorState> }),
) => void;

export function buildFileStateActions(set: StoreSet) {
  return {
    setFileState: (path: string, state: FileEditorState) => {
      set((prev) => {
        const m = new Map(prev.openFiles);
        m.set(path, state);
        return { openFiles: m };
      });
    },
    updateFileState: (path: string, updates: Partial<FileEditorState>) => {
      set((prev) => {
        const e = prev.openFiles.get(path);
        if (!e) return prev as { openFiles: Map<string, FileEditorState> };
        const m = new Map(prev.openFiles);
        m.set(path, { ...e, ...updates });
        return { openFiles: m };
      });
    },
    removeFileState: (path: string) => {
      set((prev) => {
        const m = new Map(prev.openFiles);
        m.delete(path);
        return { openFiles: m };
      });
    },
    clearFileStates: () => {
      set({ openFiles: new Map() });
    },
  };
}
