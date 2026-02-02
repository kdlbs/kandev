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

// Internal storage key for plan notifications (not exported - encapsulated)
const PLAN_NOTIFICATION_KEY = 'kandev.plan.lastSeenByTask';

/**
 * Plan notification state - tracks when user last viewed each task's plan
 * Key is taskId, value is the plan's updated_at timestamp when last viewed
 */
export type PlanNotificationState = Record<string, string | null>;

/**
 * Get the plan notification state from localStorage
 * @returns Record of taskId -> last seen plan update timestamp
 */
export function getPlanNotificationState(): PlanNotificationState {
  return getLocalStorage(PLAN_NOTIFICATION_KEY, {} as PlanNotificationState);
}

/**
 * Set the last seen timestamp for a specific task's plan
 * @param taskId - The task ID
 * @param timestamp - The plan's updated_at timestamp when viewed (or null to clear)
 */
export function setPlanLastSeen(taskId: string, timestamp: string | null): void {
  const state = getPlanNotificationState();
  if (timestamp === null) {
    delete state[taskId];
  } else {
    state[taskId] = timestamp;
  }
  setLocalStorage(PLAN_NOTIFICATION_KEY, state);
}

/**
 * Get the last seen timestamp for a specific task's plan
 * @param taskId - The task ID
 * @returns The last seen timestamp, or null if never viewed
 */
export function getPlanLastSeen(taskId: string): string | null {
  const state = getPlanNotificationState();
  return state[taskId] ?? null;
}
