import { create } from "zustand";
import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";

export type ContextFile = { path: string; name: string; pinned?: boolean };

type ContextFilesStore = {
  filesBySessionId: Record<string, ContextFile[]>;
  addFile: (sessionId: string, file: ContextFile) => void;
  removeFile: (sessionId: string, path: string) => void;
  toggleFile: (sessionId: string, file: ContextFile) => void;
  /** Set pinned=false on an existing file. */
  unpinFile: (sessionId: string, path: string) => void;
  /** Remove all files where pinned is not true. */
  clearEphemeral: (sessionId: string) => void;
  /** Remove all files for a session (for cleanup). */
  clearSession: (sessionId: string) => void;
  /** Load persisted files from sessionStorage into the store. Safe to call multiple times. */
  hydrateSession: (sessionId: string) => void;
};

const STORAGE_PREFIX = "kandev.contextFiles.";

function persistFiles(sessionId: string, files: ContextFile[]) {
  setSessionStorage(
    `${STORAGE_PREFIX}${sessionId}`,
    files.map((f) => ({ path: f.path, name: f.name, pinned: f.pinned ?? false })),
  );
}

function hydrateFiles(sessionId: string): ContextFile[] {
  return getSessionStorage<ContextFile[]>(`${STORAGE_PREFIX}${sessionId}`, []);
}

export const useContextFilesStore = create<ContextFilesStore>((set, get) => ({
  filesBySessionId: {},

  addFile: (sessionId, file) => {
    set((state) => {
      const existing = state.filesBySessionId[sessionId] ?? [];
      const idx = existing.findIndex((f) => f.path === file.path);
      if (idx >= 0) {
        // Already exists — upgrade to pinned if requested, never downgrade
        const current = existing[idx];
        if (file.pinned && !current.pinned) {
          const updated = [...existing];
          updated[idx] = { ...current, pinned: true };
          persistFiles(sessionId, updated);
          return { filesBySessionId: { ...state.filesBySessionId, [sessionId]: updated } };
        }
        return state;
      }
      const updated = [...existing, file];
      persistFiles(sessionId, updated);
      return { filesBySessionId: { ...state.filesBySessionId, [sessionId]: updated } };
    });
  },

  removeFile: (sessionId, path) => {
    set((state) => {
      const existing = state.filesBySessionId[sessionId] ?? [];
      const updated = existing.filter((f) => f.path !== path);
      persistFiles(sessionId, updated);
      return { filesBySessionId: { ...state.filesBySessionId, [sessionId]: updated } };
    });
  },

  toggleFile: (sessionId, file) => {
    const existing = get().filesBySessionId[sessionId] ?? [];
    if (existing.some((f) => f.path === file.path)) {
      get().removeFile(sessionId, file.path);
    } else {
      get().addFile(sessionId, file);
    }
  },

  unpinFile: (sessionId, path) => {
    set((state) => {
      const existing = state.filesBySessionId[sessionId] ?? [];
      const idx = existing.findIndex((f) => f.path === path);
      if (idx < 0) return state;
      const updated = [...existing];
      updated[idx] = { ...updated[idx], pinned: false };
      persistFiles(sessionId, updated);
      return { filesBySessionId: { ...state.filesBySessionId, [sessionId]: updated } };
    });
  },

  clearEphemeral: (sessionId) => {
    set((state) => {
      const existing = state.filesBySessionId[sessionId] ?? [];
      const updated = existing.filter((f) => f.pinned === true);
      persistFiles(sessionId, updated);
      return { filesBySessionId: { ...state.filesBySessionId, [sessionId]: updated } };
    });
  },

  clearSession: (sessionId) => {
    set((state) => {
      const { [sessionId]: _removed, ...rest } = state.filesBySessionId;
      void _removed;
      return { filesBySessionId: rest };
    });
  },

  hydrateSession: (sessionId) => {
    // Skip if already hydrated
    if (get().filesBySessionId[sessionId]) return;
    const hydrated = hydrateFiles(sessionId);
    if (hydrated.length > 0) {
      set((s) => ({ filesBySessionId: { ...s.filesBySessionId, [sessionId]: hydrated } }));
    }
  },
}));
