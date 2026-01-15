type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

export function getLocalStorage<T extends JsonValue>(key: string, fallback: T): T {
  if (typeof window === 'undefined') return fallback;
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

export function setLocalStorage<T extends JsonValue>(key: string, value: T): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Ignore write failures (storage full, blocked, etc.)
  }
}

export function removeLocalStorage(key: string): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Ignore removal failures.
  }
}

// Internal storage keys for kanban preview (not exported - encapsulated)
const KANBAN_PREVIEW_KEYS = {
  OPEN: 'kandev.kanban.preview.open',
  WIDTH: 'kandev.kanban.preview.width',
  SELECTED_TASK: 'kandev.kanban.preview.selectedTask',
} as const;

// Kanban preview state type
export interface KanbanPreviewState {
  isOpen: boolean;
  previewWidthPx: number;
  selectedTaskId: string | null;
}

/**
 * Get the kanban preview state from localStorage
 * @param defaults - Default values to use if not found in localStorage
 * @returns The kanban preview state
 */
export function getKanbanPreviewState(defaults: KanbanPreviewState): KanbanPreviewState {
  return {
    isOpen: getLocalStorage(KANBAN_PREVIEW_KEYS.OPEN, defaults.isOpen),
    previewWidthPx: getLocalStorage(KANBAN_PREVIEW_KEYS.WIDTH, defaults.previewWidthPx),
    selectedTaskId: getLocalStorage(KANBAN_PREVIEW_KEYS.SELECTED_TASK, defaults.selectedTaskId),
  };
}

/**
 * Set the kanban preview state in localStorage
 * @param state - Partial state to update (only provided fields are updated)
 */
export function setKanbanPreviewState(state: Partial<KanbanPreviewState>): void {
  if (state.isOpen !== undefined) {
    setLocalStorage(KANBAN_PREVIEW_KEYS.OPEN, state.isOpen);
  }
  if (state.previewWidthPx !== undefined) {
    setLocalStorage(KANBAN_PREVIEW_KEYS.WIDTH, state.previewWidthPx);
  }
  if (state.selectedTaskId !== undefined) {
    if (state.selectedTaskId === null) {
      removeLocalStorage(KANBAN_PREVIEW_KEYS.SELECTED_TASK);
    } else {
      setLocalStorage(KANBAN_PREVIEW_KEYS.SELECTED_TASK, state.selectedTaskId);
    }
  }
}
