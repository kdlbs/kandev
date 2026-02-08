import { create } from 'zustand';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';

export type ColumnId = 'left' | 'chat' | 'right' | 'preview' | 'document';

export type LayoutPreset =
  | 'default'
  | 'preview'
  | 'preview-with-right'
  | 'document'
  | 'document-with-right'
  | 'chat-only'
  | 'custom';

export type LayoutState = {
  left: boolean;
  chat: boolean;
  right: boolean;
  preview: boolean;
  document: boolean;
};

type LayoutStateBySession = {
  columnsBySessionId: Record<string, LayoutState>;
  currentPresetBySessionId: Record<string, LayoutPreset>;
  previousStateBySessionId: Record<string, LayoutState | null>;
};

type LayoutStore = LayoutStateBySession & {
  applyPreset: (sessionId: string, preset: LayoutPreset) => void;
  openPreview: (sessionId: string) => void;
  closePreview: (sessionId: string) => void;
  openDocument: (sessionId: string) => void;
  closeDocument: (sessionId: string) => void;
  toggleColumn: (sessionId: string, column: ColumnId) => void;
  showColumn: (sessionId: string, column: ColumnId) => void;
  hideColumn: (sessionId: string, column: ColumnId) => void;
  toggleRightPanel: (sessionId: string) => void;
  setColumns: (sessionId: string, columns: Partial<LayoutState>) => void;
  isVisible: (sessionId: string, column: ColumnId) => boolean;
  reset: (sessionId: string) => void;
};

const PRESETS: Record<LayoutPreset, Partial<LayoutState>> = {
  default: { left: true, chat: true, right: true, preview: false, document: false },
  preview: { left: false, chat: true, right: false, preview: true, document: false },
  'preview-with-right': { left: false, chat: true, right: true, preview: true, document: false },
  document: { left: true, chat: true, right: false, preview: false, document: true },
  'document-with-right': { left: true, chat: true, right: true, preview: false, document: true },
  'chat-only': { left: false, chat: true, right: false, preview: false, document: false },
  custom: {},
};

const DEFAULT_STATE: LayoutState = {
  left: true,
  chat: true,
  right: true,
  preview: false,
  document: false,
};

const detectPreset = (state: LayoutState): LayoutPreset => {
  for (const [preset, config] of Object.entries(PRESETS)) {
    if (preset === 'custom') continue;
    const matches = Object.entries(config).every(
      ([key, value]) => state[key as ColumnId] === value
    );
    if (matches) return preset as LayoutPreset;
  }
  return 'custom';
};

const STORAGE_KEY = 'layout-columns-by-session';

const loadPersistedState = (): Record<string, LayoutState> => {
  const stored = getLocalStorage(STORAGE_KEY, {} as Record<string, LayoutState>);
  return stored || {};
};

const persistState = (columnsBySessionId: Record<string, LayoutState>) => {
  setLocalStorage(STORAGE_KEY, columnsBySessionId);
};

export const useLayoutStore = create<LayoutStore>((set, get) => ({
  columnsBySessionId: loadPersistedState(),
  currentPresetBySessionId: {},
  previousStateBySessionId: {},

  applyPreset: (sessionId, preset) => {
    if (preset === 'custom') return;
    set((state) => {
      const current = state.columnsBySessionId[sessionId] ?? DEFAULT_STATE;
      const newColumnsBySessionId = {
        ...state.columnsBySessionId,
        [sessionId]: { ...current, ...PRESETS[preset] } as LayoutState,
      };
      persistState(newColumnsBySessionId);
      return {
        previousStateBySessionId: {
          ...state.previousStateBySessionId,
          [sessionId]: {
            left: current.left,
            chat: current.chat,
            right: current.right,
            preview: current.preview,
            document: current.document,
          },
        },
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: preset,
        },
      };
    });
  },

  openPreview: (sessionId) => {
    get().applyPreset(sessionId, 'preview-with-right');
  },

  openDocument: (sessionId) => {
    get().applyPreset(sessionId, 'document-with-right');
  },

  closeDocument: (sessionId) => {
    set((state) => {
      const previous = state.previousStateBySessionId[sessionId];
      if (previous && !previous.document) {
        const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: previous };
        persistState(newColumnsBySessionId);
        return {
          columnsBySessionId: newColumnsBySessionId,
          currentPresetBySessionId: {
            ...state.currentPresetBySessionId,
            [sessionId]: detectPreset(previous),
          },
          previousStateBySessionId: { ...state.previousStateBySessionId, [sessionId]: null },
        };
      }
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: DEFAULT_STATE };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: 'default',
        },
        previousStateBySessionId: { ...state.previousStateBySessionId, [sessionId]: null },
      };
    });
  },

  closePreview: (sessionId) => {
    set((state) => {
      const previous = state.previousStateBySessionId[sessionId];
      if (previous && !previous.preview) {
        const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: previous };
        persistState(newColumnsBySessionId);
        return {
          columnsBySessionId: newColumnsBySessionId,
          currentPresetBySessionId: {
            ...state.currentPresetBySessionId,
            [sessionId]: detectPreset(previous),
          },
          previousStateBySessionId: { ...state.previousStateBySessionId, [sessionId]: null },
        };
      }
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: DEFAULT_STATE };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: 'default',
        },
        previousStateBySessionId: { ...state.previousStateBySessionId, [sessionId]: null },
      };
    });
  },

  toggleColumn: (sessionId, column) => {
    set((state) => {
      const current = state.columnsBySessionId[sessionId] ?? DEFAULT_STATE;
      const next = { ...current, [column]: !current[column] } as LayoutState;
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: next };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: detectPreset(next),
        },
      };
    });
  },

  showColumn: (sessionId, column) => {
    set((state) => {
      const current = state.columnsBySessionId[sessionId] ?? DEFAULT_STATE;
      if (current[column]) return state;
      const next = { ...current, [column]: true } as LayoutState;
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: next };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: detectPreset(next),
        },
      };
    });
  },

  hideColumn: (sessionId, column) => {
    set((state) => {
      const current = state.columnsBySessionId[sessionId] ?? DEFAULT_STATE;
      if (!current[column]) return state;
      const next = { ...current, [column]: false } as LayoutState;
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: next };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: detectPreset(next),
        },
      };
    });
  },

  toggleRightPanel: (sessionId) => {
    set((state) => {
      const current = state.columnsBySessionId[sessionId] ?? DEFAULT_STATE;
      const next = { ...current, right: !current.right } as LayoutState;
      let preset: LayoutPreset;
      if (current.preview || next.preview) {
        preset = next.right ? 'preview-with-right' : 'preview';
      } else if (current.document || next.document) {
        preset = next.right ? 'document-with-right' : 'document';
      } else {
        preset = detectPreset(next);
      }
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: next };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: preset,
        },
      };
    });
  },

  setColumns: (sessionId, columns) => {
    set((state) => {
      const current = state.columnsBySessionId[sessionId] ?? DEFAULT_STATE;
      const next = { ...current, ...columns } as LayoutState;
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: next };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: detectPreset(next),
        },
      };
    });
  },

  isVisible: (sessionId, column) => {
    const state = get().columnsBySessionId[sessionId];
    if (!state) return column !== 'preview' && column !== 'document';
    return state[column];
  },

  reset: (sessionId) => {
    set((state) => {
      const newColumnsBySessionId = { ...state.columnsBySessionId, [sessionId]: DEFAULT_STATE };
      persistState(newColumnsBySessionId);
      return {
        columnsBySessionId: newColumnsBySessionId,
        currentPresetBySessionId: {
          ...state.currentPresetBySessionId,
          [sessionId]: 'default',
        },
        previousStateBySessionId: { ...state.previousStateBySessionId, [sessionId]: null },
      };
    });
  },
}));
