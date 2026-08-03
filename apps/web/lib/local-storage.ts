import { setWalkthroughLastSeen } from "@/lib/walkthrough-notification-storage";

type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

// Session Storage helpers (cleared when browser tab closes)
/** Read and JSON-parse a `sessionStorage` value, returning `fallback` when
 *  absent, unparseable, or storage is unavailable (SSR, blocked). */
export function getSessionStorage<T extends JsonValue>(key: string, fallback: T): T {
  if (typeof window === "undefined") return fallback;
  try {
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

/** JSON-serialize and write a value to `sessionStorage`, silently ignoring
 *  write failures (storage full, blocked, SSR). */
export function setSessionStorage<T extends JsonValue>(key: string, value: T): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Ignore write failures (storage full, blocked, etc.)
  }
}

// Local Storage helpers (persists across browser sessions)
/** Read and JSON-parse a `localStorage` value, returning `fallback` when
 *  absent, unparseable, or storage is unavailable (SSR, blocked). */
export function getLocalStorage<T extends JsonValue>(key: string, fallback: T): T {
  if (typeof window === "undefined") return fallback;
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

/** JSON-serialize and write a value to `localStorage`, silently ignoring
 *  write failures (storage full, blocked, SSR). */
export function setLocalStorage<T extends JsonValue>(key: string, value: T): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Ignore write failures (storage full, blocked, etc.)
  }
}

/** Remove a `sessionStorage` key, silently ignoring removal failures. */
export function removeSessionStorage(key: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(key);
  } catch {
    // Ignore removal failures.
  }
}

/** Remove a `localStorage` key, silently ignoring removal failures. */
export function removeLocalStorage(key: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Ignore removal failures.
  }
}

// PR panel "offered" flag — tracks whether the conditional review panel was
// shown for a session. If the user closes it, we respect that dismissal for
// the rest of the browser tab session.
const PR_PANEL_OFFERED_PREFIX = "kandev.pr-panel-offered.";

/** Whether the conditional review panel was already offered for this session. */
export function wasPRPanelOffered(sessionId: string): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.sessionStorage.getItem(`${PR_PANEL_OFFERED_PREFIX}${sessionId}`) === "1";
  } catch {
    return false;
  }
}

/** Record that the conditional review panel was offered for this session. */
export function markPRPanelOffered(sessionId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(`${PR_PANEL_OFFERED_PREFIX}${sessionId}`, "1");
  } catch {
    // Ignore write failures.
  }
}

// Internal storage keys for kanban preview (not exported - encapsulated)
const KANBAN_PREVIEW_KEYS = {
  OPEN: "kandev.kanban.preview.open",
  WIDTH: "kandev.kanban.preview.width",
  SELECTED_TASK: "kandev.kanban.preview.selectedTask",
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

const PLAN_NOTIFICATION_KEY = "kandev.plan.lastSeenByTask";

export type PlanNotificationState = Record<string, string | null>;

/** Read the map of task ID to last-seen plan-update timestamp (localStorage). */
export function getPlanNotificationState(): PlanNotificationState {
  return getLocalStorage(PLAN_NOTIFICATION_KEY, {} as PlanNotificationState);
}

/** Record (or clear, when `timestamp` is `null`) the last-seen plan-update
 *  timestamp for a task. */
export function setPlanLastSeen(taskId: string, timestamp: string | null): void {
  const state = getPlanNotificationState();
  if (timestamp === null) {
    delete state[taskId];
  } else {
    state[taskId] = timestamp;
  }
  setLocalStorage(PLAN_NOTIFICATION_KEY, state);
}

/** Read the last-seen plan-update timestamp for a task, or `null` if never recorded. */
export function getPlanLastSeen(taskId: string): string | null {
  const state = getPlanNotificationState();
  return state[taskId] ?? null;
}

// Internal storage key for center panel tab (uses sessionStorage)
const CENTER_PANEL_TAB_KEY = "kandev.centerPanel.tab";

/**
 * Get the saved center panel tab from sessionStorage
 * @param fallback - Default tab if not found
 * @returns The saved tab id
 */
export function getCenterPanelTab(fallback: string): string {
  if (typeof window === "undefined") return fallback;
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
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(CENTER_PANEL_TAB_KEY, JSON.stringify(tab));
  } catch {
    // Ignore write failures
  }
}

// Internal storage keys for files panel (uses sessionStorage for per-tab persistence)
const FILES_PANEL_KEYS = {
  EXPANDED: "kandev.filesPanel.expanded",
  SCROLL: "kandev.filesPanel.scroll",
} as const;

/**
 * Get the saved expanded paths for file browser
 * @param sessionId - The session ID
 * @returns Array of expanded folder paths
 */
export function getFilesPanelExpandedPaths(sessionId: string): string[] {
  if (typeof window === "undefined") return [];
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
  if (typeof window === "undefined") return;
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
  if (typeof window === "undefined") return 0;
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
  if (typeof window === "undefined") return;
  try {
    const key = `${FILES_PANEL_KEYS.SCROLL}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(position));
  } catch {
    // Ignore write failures
  }
}

// --- Dockview per-session layout (sessionStorage) ---
// Dockview layout is keyed by `taskEnvironmentId` so sessions sharing a task
// env reuse one layout (the env owns the workspace, terminals, files, etc.).
//
// v2: bumped when the sidebar/right viewport-proportional caps shipped.
// Layouts saved by older builds capture column widths that may exceed the
// new initial-default caps; loading them would resurface the very behaviour
// users complained about (sidebar "maxed out" by default after upgrade).
// Bumping the prefix invalidates legacy saves so every env opens at the
// preset defaults once, then resumes per-env persistence. Bumped to v3 to
// discard layouts captured with the now-removed dockview sidebar column.
const DOCKVIEW_ENV_LAYOUT_PREFIX = "kandev.dockview.env-layout-v3.";

// A serialized Dockview layout is geometry, not evidence of an intentional
// pixel preference. Keep that preference separately so automatic restores can
// recompute responsive defaults while genuine sash drags remain per-env.
const DOCKVIEW_MANUAL_RIGHT_WIDTH_PREFIX = "kandev.dockview.manual-right-width-v1.";

/**
 * Get the saved dockview layout for a task environment.
 * Returns null if not found.
 */
export function getEnvLayout(envId: string): object | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(`${DOCKVIEW_ENV_LAYOUT_PREFIX}${envId}`);
    if (!raw) return null;
    return JSON.parse(raw) as object;
  } catch {
    return null;
  }
}

/** Save the dockview layout for a task environment. */
export function setEnvLayout(envId: string, layout: object): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(`${DOCKVIEW_ENV_LAYOUT_PREFIX}${envId}`, JSON.stringify(layout));
  } catch {
    // Ignore write failures (storage full, blocked, etc.)
  }
}

/** Read the manually-dragged right-panel width for a task env, or `null` if
 *  never set or invalid (sessionStorage). */
export function getManualRightWidth(envId: string | null): number | null {
  if (!envId) return null;
  const value = getSessionStorage<number | null>(
    `${DOCKVIEW_MANUAL_RIGHT_WIDTH_PREFIX}${envId}`,
    null,
  );
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? Math.round(value)
    : null;
}

/** Record a genuine sash-drag width for the right panel of a task env. */
export function setManualRightWidth(envId: string | null, width: number): void {
  if (!envId || !Number.isFinite(width) || width <= 0) return;
  setSessionStorage(`${DOCKVIEW_MANUAL_RIGHT_WIDTH_PREFIX}${envId}`, Math.round(width));
}

/** Clear the manually-dragged right-panel width for a task env, reverting
 *  to the responsive default. */
export function clearManualRightWidth(envId: string | null): void {
  if (!envId) return;
  removeSessionStorage(`${DOCKVIEW_MANUAL_RIGHT_WIDTH_PREFIX}${envId}`);
}

// --- Dockview global left-sidebar width (localStorage) ---
// The LEFT sidebar width is a single GLOBAL preference shared across every
// task env (unlike the per-env layout above, which keys widths by envId).
// Stores the user's raw, unclamped width — clamping to the current screen
// happens at apply time. Written only by a genuine sash drag; read by every
// layout build/restore/switch via getPinnedWidth.
const DOCKVIEW_GLOBAL_SIDEBAR_WIDTH_KEY = "kandev.dockview.sidebar-width";

/** Read the global left-sidebar width, or `null` if never set or invalid
 *  (localStorage, shared across every task env). */
export function getGlobalSidebarWidth(): number | null {
  const v = getLocalStorage<number | null>(DOCKVIEW_GLOBAL_SIDEBAR_WIDTH_KEY, null);
  return typeof v === "number" && Number.isFinite(v) && v > 0 ? v : null;
}

/** Record a genuine sash-drag width for the global left sidebar. */
export function setGlobalSidebarWidth(width: number): void {
  if (!Number.isFinite(width) || width <= 0) return;
  setLocalStorage(DOCKVIEW_GLOBAL_SIDEBAR_WIDTH_KEY, Math.round(width));
}

/** Clear the global left-sidebar width, reverting to the responsive default. */
export function clearGlobalSidebarWidth(): void {
  removeLocalStorage(DOCKVIEW_GLOBAL_SIDEBAR_WIDTH_KEY);
}

// --- Dockview per-env maximize state (sessionStorage) ---
// v3: bumped in lockstep with DOCKVIEW_ENV_LAYOUT_PREFIX. The maximize blob
// references the pre-maximize layout, which can carry the same oversized
// widths as the env layout.
const DOCKVIEW_ENV_MAXIMIZE_PREFIX = "kandev.dockview.env-maximize-v3.";

export type EnvMaximizeState = {
  /** The pre-maximize (normal) layout to restore on exit-maximize. */
  preMaximizeLayout: object;
  /** Native dockview JSON (api.toJSON()) for the maximized layout. */
  maximizedDockviewJson: object;
};

/** Type guard validating a parsed value has the shape of {@link EnvMaximizeState}. */
function isEnvMaximizeState(value: unknown): value is EnvMaximizeState {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.preMaximizeLayout === "object" &&
    v.preMaximizeLayout !== null &&
    typeof v.maximizedDockviewJson === "object" &&
    v.maximizedDockviewJson !== null
  );
}

/** Read the saved pre-maximize layout + maximized dockview JSON for a task
 *  env, or `null` if absent or malformed. */
export function getEnvMaximizeState(envId: string): EnvMaximizeState | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(`${DOCKVIEW_ENV_MAXIMIZE_PREFIX}${envId}`);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return isEnvMaximizeState(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

/** Save the pre-maximize layout + maximized dockview JSON for a task env,
 *  so exit-maximize can restore it. */
export function setEnvMaximizeState(envId: string, state: EnvMaximizeState): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(`${DOCKVIEW_ENV_MAXIMIZE_PREFIX}${envId}`, JSON.stringify(state));
  } catch {
    // Ignore write failures
  }
}

/** Clear the saved maximize state for a task env (e.g. on layout reset). */
export function removeEnvMaximizeState(envId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(`${DOCKVIEW_ENV_MAXIMIZE_PREFIX}${envId}`);
  } catch {
    // Ignore
  }
}

// PR merged banner dismissal — per-task, survives reload + task switch within
// the tab session, resets on tab close.
const PR_MERGED_BANNER_DISMISSED_PREFIX = "kandev.pr-merged-banner-dismissed.";

/** Whether the user dismissed the PR-merged banner for this task. */
export function wasPRMergedBannerDismissed(taskId: string): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.sessionStorage.getItem(`${PR_MERGED_BANNER_DISMISSED_PREFIX}${taskId}`) === "1";
  } catch {
    return false;
  }
}

/** Record that the user dismissed the PR-merged banner for this task. */
export function markPRMergedBannerDismissed(taskId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(`${PR_MERGED_BANNER_DISMISSED_PREFIX}${taskId}`, "1");
  } catch {
    // Ignore write failures
  }
}

// PR closed banner dismissal — same lifetime as the merged banner: per-task,
// survives reload + task switch within the tab session, resets on tab close.
const PR_CLOSED_BANNER_DISMISSED_PREFIX = "kandev.pr-closed-banner-dismissed.";

/** Whether the user dismissed the PR-closed banner for this task. */
export function wasPRClosedBannerDismissed(taskId: string): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.sessionStorage.getItem(`${PR_CLOSED_BANNER_DISMISSED_PREFIX}${taskId}`) === "1";
  } catch {
    return false;
  }
}

/** Record that the user dismissed the PR-closed banner for this task. */
export function markPRClosedBannerDismissed(taskId: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(`${PR_CLOSED_BANNER_DISMISSED_PREFIX}${taskId}`, "1");
  } catch {
    // Ignore write failures
  }
}

// Internal storage keys for open file tabs
const OPEN_FILES_KEY = "kandev.openFiles";
const ACTIVE_TAB_KEY = "kandev.activeTab";

/**
 * Minimal tab info stored in sessionStorage (no content - reloaded on restore).
 * `pinned` distinguishes user-pinned tabs (restored always) from the single
 * preview tab (restored as preview).
 */
export interface StoredFileTab {
  path: string;
  name: string;
  /** Multi-repo subpath (repository_name) so a restored tab re-fetches its
   *  content under the right repository after a refresh. */
  repo?: string;
  markdownPreview?: boolean;
  pinned?: boolean;
}

/**
 * Get the saved open file tabs for a session.
 *
 * Legacy records (written before the preview-tab feature) have no `pinned`
 * field. We treat them as pinned so the user's previously-open files don't
 * suddenly collapse to a single preview after upgrading.
 */
export function getOpenFileTabs(sessionId: string): StoredFileTab[] {
  if (typeof window === "undefined") return [];
  try {
    const key = `${OPEN_FILES_KEY}.${sessionId}`;
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as StoredFileTab[];
    if (!Array.isArray(parsed)) return [];
    // At most one tab can be the preview; keep the last one flagged preview and
    // treat every other record as pinned. Records with `pinned: undefined` are
    // legacy → pin them so we don't lose them.
    let previewSeen = false;
    const normalized: StoredFileTab[] = [];
    for (let i = parsed.length - 1; i >= 0; i--) {
      const t = parsed[i];
      if (!t) continue;
      const isPinned = t.pinned === true || t.pinned === undefined;
      if (isPinned) {
        normalized.unshift({ ...t, pinned: true });
      } else if (!previewSeen) {
        previewSeen = true;
        normalized.unshift({ ...t, pinned: false });
      }
    }
    return normalized;
  } catch {
    return [];
  }
}

/** Save the open file tabs for a session. */
export function setOpenFileTabs(sessionId: string, tabs: StoredFileTab[]): void {
  if (typeof window === "undefined") return;
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
  if (typeof window === "undefined") return fallback;
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
  if (typeof window === "undefined") return;
  try {
    const key = `${ACTIVE_TAB_KEY}.${sessionId}`;
    window.sessionStorage.setItem(key, JSON.stringify(tabId));
  } catch {
    // Ignore write failures
  }
}

// --- Chat draft persistence (sessionStorage, per task) ---

const CHAT_DRAFT_TEXT_KEY = "kandev.chatDraft.text";
const CHAT_DRAFT_CONTENT_KEY = "kandev.chatDraft.content";
const CHAT_DRAFT_ATTACHMENTS_KEY = "kandev.chatDraft.attachments";
const CHAT_INPUT_HEIGHT_KEY = "kandev.chatInput.height";

/** Stored attachment — same as FileAttachment but without `preview` (reconstructed on load) */
type StoredFileAttachment = {
  id: string;
  data: string;
  mimeType: string;
  fileName: string;
  size: number;
  isImage: boolean;
  deliveryMode?: "prompt" | "path";
};

/** Read the draft chat message text for a session, or `""` if none saved. */
export function getChatDraftText(sessionId: string): string {
  return getSessionStorage(`${CHAT_DRAFT_TEXT_KEY}.${sessionId}`, "");
}

/** Save (or clear, when empty) the draft chat message text for a session. */
export function setChatDraftText(sessionId: string, text: string): void {
  if (text === "") {
    removeSessionStorage(`${CHAT_DRAFT_TEXT_KEY}.${sessionId}`);
  } else {
    setSessionStorage(`${CHAT_DRAFT_TEXT_KEY}.${sessionId}`, text);
  }
}

/** TipTap editor JSON — preserves rich content (mentions, code blocks, etc.) */
export function getChatDraftContent(sessionId: string): unknown {
  return getSessionStorage<JsonValue | null>(`${CHAT_DRAFT_CONTENT_KEY}.${sessionId}`, null);
}

/** Save (or clear, when falsy) the draft TipTap editor JSON for a session. */
export function setChatDraftContent(sessionId: string, content: unknown): void {
  if (!content) {
    removeSessionStorage(`${CHAT_DRAFT_CONTENT_KEY}.${sessionId}`);
  } else {
    setSessionStorage(`${CHAT_DRAFT_CONTENT_KEY}.${sessionId}`, content as JsonValue);
  }
}

/** Read the draft chat attachments for a session (previews reconstructed by
 *  the caller, not persisted). */
export function getChatDraftAttachments(sessionId: string): StoredFileAttachment[] {
  return getSessionStorage<StoredFileAttachment[]>(
    `${CHAT_DRAFT_ATTACHMENTS_KEY}.${sessionId}`,
    [],
  );
}

/** Coerce a stored attachment delivery-mode value, falling back for anything
 *  other than `"prompt"` or `"path"`. */
function normalizeAttachmentDeliveryMode(
  value: unknown,
  fallback: "prompt" | "path",
): "prompt" | "path" {
  return value === "prompt" || value === "path" ? value : fallback;
}

/** Save (or clear, when empty) the draft chat attachments for a session,
 *  stripping `preview` to halve storage cost. */
export function setChatDraftAttachments(
  sessionId: string,
  attachments: Array<{
    id: string;
    data: string;
    mimeType: string;
    fileName: string;
    size: number;
    isImage: boolean;
    deliveryMode?: "prompt" | "path";
    preview?: string;
  }>,
): void {
  if (attachments.length === 0) {
    removeSessionStorage(`${CHAT_DRAFT_ATTACHMENTS_KEY}.${sessionId}`);
  } else {
    // Strip `preview` to halve storage cost — reconstructed on load for images
    const stored: StoredFileAttachment[] = attachments.map(
      ({ id, data, mimeType, fileName, size, isImage, deliveryMode }) => ({
        id,
        data,
        mimeType,
        fileName,
        size,
        isImage,
        deliveryMode,
      }),
    );
    setSessionStorage(`${CHAT_DRAFT_ATTACHMENTS_KEY}.${sessionId}`, stored);
  }
}

/**
 * Reconstruct the `preview` data URL from stored attachment data (images only).
 */
export function restoreAttachmentPreview(
  att: StoredFileAttachment,
): StoredFileAttachment & { deliveryMode: "prompt" | "path"; preview?: string } {
  if (att.isImage) {
    return {
      ...att,
      deliveryMode: normalizeAttachmentDeliveryMode(att.deliveryMode, "prompt"),
      preview: `data:${att.mimeType};base64,${att.data}`,
    };
  }
  return { ...att, deliveryMode: normalizeAttachmentDeliveryMode(att.deliveryMode, "path") };
}

/** Read the saved chat composer height for a session, or `null` if never set. */
export function getChatInputHeight(sessionId: string): number | null {
  return getSessionStorage<number | null>(`${CHAT_INPUT_HEIGHT_KEY}.${sessionId}`, null);
}

/** Save the chat composer height for a session. */
export function setChatInputHeight(sessionId: string, height: number): void {
  setSessionStorage(`${CHAT_INPUT_HEIGHT_KEY}.${sessionId}`, height);
}

// --- Transcript auto-scroll toggle (sessionStorage, per session) ---
// Survives reload + task switch within the tab session, resets on tab close.
const AUTO_SCROLL_ENABLED_PREFIX = "kandev.transcript-auto-scroll-enabled.";

/** Read the per-session auto-scroll preference persisted by
 *  {@link setStoredAutoScrollEnabled}, or `null` if never recorded. */
export function getStoredAutoScrollEnabled(sessionId: string): boolean | null {
  return getSessionStorage<boolean | null>(`${AUTO_SCROLL_ENABLED_PREFIX}${sessionId}`, null);
}

/** Persist the per-session auto-scroll toggle preference so it survives a
 *  reload or task switch within the tab session (sessionStorage). */
export function setStoredAutoScrollEnabled(sessionId: string, enabled: boolean): void {
  setSessionStorage(`${AUTO_SCROLL_ENABLED_PREFIX}${sessionId}`, enabled);
}

const AUTO_SCROLL_TOP_PREFIX = "kandev.transcript-auto-scroll-top.";

/** Read the last scrollTop persisted by {@link setStoredAutoScrollTop} for
 *  this session, or `null` if never recorded. */
export function getStoredAutoScrollTop(sessionId: string): number | null {
  return getSessionStorage<number | null>(`${AUTO_SCROLL_TOP_PREFIX}${sessionId}`, null);
}

/** Persist the transcript's last scrollTop for this session so a disabled
 *  auto-scroll preference restores the same position after a reload or task
 *  switch within the tab session (sessionStorage). */
export function setStoredAutoScrollTop(sessionId: string, scrollTop: number): void {
  setSessionStorage(`${AUTO_SCROLL_TOP_PREFIX}${sessionId}`, scrollTop);
}

// --- Task storage cleanup ---

/**
 * Remove all session/env-scoped storage for a deleted task.
 * Call from task.deleted handler before the task is removed from state.
 */
export function cleanupTaskStorage(
  taskId: string,
  sessionIds: string[],
  envIds: string[] = [],
): void {
  // Plan notification (localStorage, keyed per task inside a Record)
  setPlanLastSeen(taskId, null);
  setWalkthroughLastSeen(taskId, null);

  // PR merged / closed banner dismissal (sessionStorage, keyed per task)
  removeSessionStorage(`${PR_MERGED_BANNER_DISMISSED_PREFIX}${taskId}`);
  removeSessionStorage(`${PR_CLOSED_BANNER_DISMISSED_PREFIX}${taskId}`);

  // Sidebar collapsed-subtask set (sessionStorage, array keyed by parent taskId)
  const collapsed = getStoredCollapsedSubtaskParents();
  if (collapsed.includes(taskId)) {
    setStoredCollapsedSubtaskParents(collapsed.filter((id) => id !== taskId));
  }

  // Env-keyed storage — dockview layout + maximize live under task envs.
  for (const envId of envIds) {
    removeEnvMaximizeState(envId);
    removeSessionStorage(`${DOCKVIEW_ENV_LAYOUT_PREFIX}${envId}`);
    clearManualRightWidth(envId);
  }

  // Session-keyed storage — drafts, files panel state, scroll, etc.
  for (const sessionId of sessionIds) {
    removeStoredQuickChatName(sessionId);
    removeSessionStorage(`${PR_PANEL_OFFERED_PREFIX}${sessionId}`);
    removeSessionStorage(`${CHAT_DRAFT_TEXT_KEY}.${sessionId}`);
    removeSessionStorage(`${CHAT_DRAFT_CONTENT_KEY}.${sessionId}`);
    removeSessionStorage(`${CHAT_DRAFT_ATTACHMENTS_KEY}.${sessionId}`);
    removeSessionStorage(`${CHAT_INPUT_HEIGHT_KEY}.${sessionId}`);
    removeSessionStorage(`${FILES_PANEL_KEYS.EXPANDED}.${sessionId}`);
    removeSessionStorage(`${FILES_PANEL_KEYS.SCROLL}.${sessionId}`);
    removeSessionStorage(`${OPEN_FILES_KEY}.${sessionId}`);
    removeSessionStorage(`${ACTIVE_TAB_KEY}.${sessionId}`);
    removeSessionStorage(`kandev.contextFiles.${sessionId}`);
    removeSessionStorage(`kandev.comments.${sessionId}`);
    removeSessionStorage(`kandev.messageFavorites.${sessionId}`);
    removeSessionStorage(`${AUTO_SCROLL_ENABLED_PREFIX}${sessionId}`);
    removeSessionStorage(`${AUTO_SCROLL_TOP_PREFIX}${sessionId}`);
  }
}

// --- Quick chat custom names (localStorage, global) ---
//
// Local-only: not persisted to the backend. Keyed by sessionId so the rename
// follows the underlying chat across reloads but vanishes if the user clears
// browser storage. The backend task title remains the auto-generated name.

const QUICK_CHAT_NAMES_KEY = "kandev.quickChat.names";

/** Read the map of session ID to user-renamed quick-chat name (localStorage). */
export function getStoredQuickChatNames(): Record<string, string> {
  const raw = getLocalStorage<Record<string, string>>(QUICK_CHAT_NAMES_KEY, {});
  if (!raw || typeof raw !== "object") return {};
  return raw;
}

/** Set (or clear, when `name` is empty) the user-renamed quick-chat name for a session. */
export function setStoredQuickChatName(sessionId: string, name: string): void {
  if (!sessionId) return;
  const all = getStoredQuickChatNames();
  if (name) all[sessionId] = name;
  else delete all[sessionId];
  setLocalStorage(QUICK_CHAT_NAMES_KEY, all);
}

/** Remove the user-renamed quick-chat name for a session. */
export function removeStoredQuickChatName(sessionId: string): void {
  if (!sessionId) return;
  const all = getStoredQuickChatNames();
  if (!(sessionId in all)) return;
  delete all[sessionId];
  setLocalStorage(QUICK_CHAT_NAMES_KEY, all);
}

// AppSidebar collapse/section/width storage helpers live in
// `./local-storage-app-sidebar` to keep this module under the line cap.

// --- Sidebar collapsed subtask parents (sessionStorage, tab-scoped) ---

const COLLAPSED_SUBTASKS_KEY = "kandev.sidebar.collapsedSubtasks";

/**
 * Get the list of parent task IDs whose subtasks are collapsed in the sidebar.
 * Tab-scoped (sessionStorage) so it survives reload/task switches but not tab close.
 */
export function getStoredCollapsedSubtaskParents(): string[] {
  const raw = getSessionStorage<string[]>(COLLAPSED_SUBTASKS_KEY, []) as unknown;
  if (!Array.isArray(raw)) return [];
  return raw.filter((id): id is string => typeof id === "string");
}

/**
 * Save the list of parent task IDs whose subtasks are collapsed in the sidebar.
 */
export function setStoredCollapsedSubtaskParents(ids: string[]): void {
  setSessionStorage(COLLAPSED_SUBTASKS_KEY, ids);
}

// --- Task creation draft persistence (sessionStorage, per workspace) ---

const TASK_CREATE_DRAFT_KEY = "kandev.taskCreateDraft";

/**
 * Draft data for task creation dialog.
 * Only persists user-entered content fields (title, description).
 * Other fields (repo, branch, agent) are handled by "last used" localStorage.
 */
export type TaskCreateDraft = {
  title: string;
  description: string;
};

/**
 * Get the saved task creation draft for a workspace.
 * @param workspaceId - The workspace ID
 * @returns The draft data, or null if no draft exists
 */
export function getTaskCreateDraft(workspaceId: string): TaskCreateDraft | null {
  if (!workspaceId) return null;
  return getSessionStorage<TaskCreateDraft | null>(`${TASK_CREATE_DRAFT_KEY}.${workspaceId}`, null);
}

/**
 * Save a task creation draft for a workspace.
 * @param workspaceId - The workspace ID
 * @param draft - The draft data to save
 */
export function setTaskCreateDraft(workspaceId: string, draft: TaskCreateDraft): void {
  if (!workspaceId) return;
  // Only save if there's actual content
  if (!draft.title.trim() && !draft.description.trim()) {
    removeTaskCreateDraft(workspaceId);
    return;
  }
  setSessionStorage(`${TASK_CREATE_DRAFT_KEY}.${workspaceId}`, draft);
}

/**
 * Remove the task creation draft for a workspace.
 * @param workspaceId - The workspace ID
 */
export function removeTaskCreateDraft(workspaceId: string): void {
  if (!workspaceId) return;
  removeSessionStorage(`${TASK_CREATE_DRAFT_KEY}.${workspaceId}`);
}
