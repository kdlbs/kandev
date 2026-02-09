type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

// Session Storage helpers (cleared when browser tab closes)
export function getSessionStorage<T extends JsonValue>(key: string, fallback: T): T {
  if (typeof window === 'undefined') return fallback;
  try {
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

export function setSessionStorage<T extends JsonValue>(key: string, value: T): void {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Ignore write failures (storage full, blocked, etc.)
  }
}

// Local Storage helpers (persists across browser sessions)
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

// Internal storage key for center panel tab (uses sessionStorage)
const CENTER_PANEL_TAB_KEY = 'kandev.centerPanel.tab';

/**
 * Get the saved center panel tab from sessionStorage
 * @param fallback - Default tab if not found
 * @returns The saved tab id
 */
export function getCenterPanelTab(fallback: string): string {
  if (typeof window === 'undefined') return fallback;
  try {
    const raw = window.sessionStorage.getItem(CENTER_PANEL_TAB_KEY);
    if (!raw) return fallback;
    return JSON.parse(raw) as string;
  } catch {
    return fallback;
  }
}

/**
 * Save the center panel tab to sessionStorage
 * @param tab - The tab id to save
 */
export function setCenterPanelTab(tab: string): void {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.setItem(CENTER_PANEL_TAB_KEY, JSON.stringify(tab));
  } catch {
    // Ignore write failures
  }
}

// Internal storage keys for files panel (uses sessionStorage for per-tab persistence)
const FILES_PANEL_KEYS = {
  TAB: 'kandev.filesPanel.tab',
  USER_SELECTED: 'kandev.filesPanel.userSelected',
  EXPANDED: 'kandev.filesPanel.expanded',
  SCROLL: 'kandev.filesPanel.scroll',
} as const;

/**
 * Get the saved files panel tab for a session
 * @param sessionId - The session ID
 * @param fallback - Default tab if not found
 * @returns The saved tab ('diff' or 'files')
 */
export function getFilesPanelTab(sessionId: string, fallback: 'diff' | 'files'): 'diff' | 'files' {
  if (typeof window === 'undefined') return fallback;
  try {
    const key = `${FILES_PANEL_KEYS.TAB}.${sessionId}`;
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return fallback;
    const value = JSON.parse(raw) as string;
    return value === 'diff' || value === 'files' ? value : fallback;
  } catch {
    return fallback;
  }
}

/**
 * Save the files panel tab for a session
 * @param sessionId - The session ID
 * @param tab - The tab to save ('diff' or 'files')
 */
export function setFilesPanelTab(sessionId: string, tab: 'diff' | 'files'): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `${FILES_PANEL_KEYS.TAB}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(tab));
  } catch {
    // Ignore write failures
  }
}

/**
 * Check if user has explicitly selected a tab for this session
 * @param sessionId - The session ID
 * @returns true if user has made a selection
 */
export function hasUserSelectedFilesPanelTab(sessionId: string): boolean {
  if (typeof window === 'undefined') return false;
  try {
    const key = `${FILES_PANEL_KEYS.USER_SELECTED}.${sessionId}`;
    return window.sessionStorage.getItem(key) === 'true';
  } catch {
    return false;
  }
}

/**
 * Mark that user has explicitly selected a tab for this session
 * @param sessionId - The session ID
 */
export function setUserSelectedFilesPanelTab(sessionId: string): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `${FILES_PANEL_KEYS.USER_SELECTED}.${sessionId}`;
    window.sessionStorage.setItem(key, 'true');
  } catch {
    // Ignore write failures
  }
}

/**
 * Get the saved expanded paths for file browser
 * @param sessionId - The session ID
 * @returns Array of expanded folder paths
 */
export function getFilesPanelExpandedPaths(sessionId: string): string[] {
  if (typeof window === 'undefined') return [];
  try {
    const key = `${FILES_PANEL_KEYS.EXPANDED}.${sessionId}`;
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return [];
    return JSON.parse(raw) as string[];
  } catch {
    return [];
  }
}

/**
 * Save the expanded paths for file browser
 * @param sessionId - The session ID
 * @param paths - Array of expanded folder paths
 */
export function setFilesPanelExpandedPaths(sessionId: string, paths: string[]): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `${FILES_PANEL_KEYS.EXPANDED}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(paths));
  } catch {
    // Ignore write failures
  }
}

/**
 * Get the saved scroll position for file browser
 * @param sessionId - The session ID
 * @returns The scroll position in pixels
 */
export function getFilesPanelScrollPosition(sessionId: string): number {
  if (typeof window === 'undefined') return 0;
  try {
    const key = `${FILES_PANEL_KEYS.SCROLL}.${sessionId}`;
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return 0;
    return JSON.parse(raw) as number;
  } catch {
    return 0;
  }
}

/**
 * Save the scroll position for file browser
 * @param sessionId - The session ID
 * @param position - The scroll position in pixels
 */
export function setFilesPanelScrollPosition(sessionId: string, position: number): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `${FILES_PANEL_KEYS.SCROLL}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(position));
  } catch {
    // Ignore write failures
  }
}

// --- Dockview per-session layout (sessionStorage) ---
const DOCKVIEW_SESSION_LAYOUT_PREFIX = 'kandev.dockview.layout.';

/**
 * Get the saved dockview layout for a session
 * @param sessionId - The session ID
 * @returns The serialized layout object, or null if not found
 */
export function getSessionLayout(sessionId: string): object | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.sessionStorage.getItem(`${DOCKVIEW_SESSION_LAYOUT_PREFIX}${sessionId}`);
    if (!raw) return null;
    return JSON.parse(raw) as object;
  } catch {
    return null;
  }
}

/**
 * Save the dockview layout for a session
 * @param sessionId - The session ID
 * @param layout - The serialized layout object from api.toJSON()
 */
export function setSessionLayout(sessionId: string, layout: object): void {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.setItem(
      `${DOCKVIEW_SESSION_LAYOUT_PREFIX}${sessionId}`,
      JSON.stringify(layout),
    );
  } catch {
    // Ignore write failures (storage full, blocked, etc.)
  }
}

// Internal storage keys for open file tabs
const OPEN_FILES_KEY = 'kandev.openFiles';
const ACTIVE_TAB_KEY = 'kandev.activeTab';

/**
 * Minimal tab info stored in sessionStorage (no content - reloaded on restore)
 */
export interface StoredFileTab {
  path: string;
  name: string;
}

/**
 * Get the saved open file tabs for a session
 * @param sessionId - The session ID
 * @returns Array of stored file tabs (path and name only)
 */
export function getOpenFileTabs(sessionId: string): StoredFileTab[] {
  if (typeof window === 'undefined') return [];
  try {
    const key = `${OPEN_FILES_KEY}.${sessionId}`;
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return [];
    return JSON.parse(raw) as StoredFileTab[];
  } catch {
    return [];
  }
}

/**
 * Save the open file tabs for a session
 * @param sessionId - The session ID
 * @param tabs - Array of tabs (only path and name are stored)
 */
export function setOpenFileTabs(sessionId: string, tabs: StoredFileTab[]): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `${OPEN_FILES_KEY}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(tabs));
  } catch {
    // Ignore write failures
  }
}

/**
 * Get the saved active tab for a session
 * @param sessionId - The session ID
 * @param fallback - Default tab if not found
 * @returns The saved active tab id (e.g., 'chat', 'plan', 'file:/path/to/file')
 */
export function getActiveTabForSession(sessionId: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback;
  try {
    const key = `${ACTIVE_TAB_KEY}.${sessionId}`;
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw) as string;
  } catch {
    return fallback;
  }
}

/**
 * Save the active tab for a session
 * @param sessionId - The session ID
 * @param tabId - The tab id to save
 */
export function setActiveTabForSession(sessionId: string, tabId: string): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `${ACTIVE_TAB_KEY}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(tabId));
  } catch {
    // Ignore write failures
  }
}
