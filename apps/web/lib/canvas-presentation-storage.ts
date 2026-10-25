import { generateUUID } from "@/lib/uuid";

export type CanvasPresentationIdentity = {
  userId: string;
  workspaceId: string;
  taskId: string;
  canvasId: string;
};

export type CanvasPresentationReason = "existing" | "restored" | "manual" | "automatic";

const CANVAS_PRESENTATION_PREFIX = "kandev.canvas-presentation.v2";
const TAB_INSTANCE_KEY = `${CANVAS_PRESENTATION_PREFIX}.tab`;
const TAB_OWNER_PREFIX = `${CANVAS_PRESENTATION_PREFIX}.owner.`;
// i18n-exempt: stable browser-storage identity, not user-facing copy.
const ANONYMOUS_CANVAS_PRESENTATION_IDENTITY = "anonymous";
const SERVER_TAB_IDENTITY = "server";
const memoryReceipts = new Set<string>();

type TabOwnerRecord = {
  pageId: string;
  active: boolean;
};

type TabIdentity = {
  tabId: string;
  pageId: string;
};

let tabIdentity: TabIdentity | null = null;

export function canvasPresentationUserId(auth: {
  mode: "disabled" | "setup" | "enabled";
  user: { id: string } | null;
}): string | null {
  if (auth.user?.id) return auth.user.id;
  return auth.mode === "disabled" ? ANONYMOUS_CANVAS_PRESENTATION_IDENTITY : null;
}

function sessionStorageOrNull(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

function localStorageOrNull(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function readTabOwner(storage: Storage, tabId: string): TabOwnerRecord | null {
  try {
    const raw = storage.getItem(`${TAB_OWNER_PREFIX}${tabId}`);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<TabOwnerRecord>;
    if (typeof parsed.pageId !== "string" || typeof parsed.active !== "boolean") return null;
    return { pageId: parsed.pageId, active: parsed.active };
  } catch {
    return null;
  }
}

function writeTabOwner(storage: Storage, tabId: string, record: TabOwnerRecord): void {
  try {
    storage.setItem(`${TAB_OWNER_PREFIX}${tabId}`, JSON.stringify(record));
  } catch {
    // Ignore private browsing, blocked storage, and quota failures.
  }
}

function writeSessionTabId(storage: Storage | null, tabId: string): void {
  if (!storage) return;
  try {
    storage.setItem(TAB_INSTANCE_KEY, tabId);
  } catch {
    // Receipts still use the in-memory fallback when session storage is blocked.
  }
}

/**
 * Return a stable identity for this tab while detecting a duplicated
 * sessionStorage namespace. A reload releases the old page owner first, while
 * a duplicated tab sees the live owner and receives a fresh namespace.
 */
function canvasPresentationTabId(): string {
  if (typeof window === "undefined") return SERVER_TAB_IDENTITY;

  const session = sessionStorageOrNull();
  let storedTabId: string | null = null;
  try {
    storedTabId = session?.getItem(TAB_INSTANCE_KEY) ?? null;
  } catch {
    storedTabId = null;
  }
  if (tabIdentity && storedTabId === tabIdentity.tabId) return tabIdentity.tabId;

  const pageId = generateUUID();
  let tabId = storedTabId ?? generateUUID();
  const local = localStorageOrNull();
  if (local) {
    const owner = readTabOwner(local, tabId);
    if (owner?.active && owner.pageId !== pageId) {
      tabId = generateUUID();
      writeSessionTabId(session, tabId);
    } else if (!storedTabId) {
      writeSessionTabId(session, tabId);
    }
    writeTabOwner(local, tabId, { pageId, active: true });
    window.addEventListener(
      "pagehide",
      (event) => {
        if (event.persisted) return;
        if (readTabOwner(local, tabId)?.pageId === pageId) {
          writeTabOwner(local, tabId, { pageId, active: false });
        }
      },
      { once: true },
    );
  } else if (!storedTabId) {
    writeSessionTabId(session, tabId);
  }

  tabIdentity = { tabId, pageId };
  return tabId;
}

/** Build the tab-local identity for one presented task canvas. */
export function canvasPresentationKey(identity: CanvasPresentationIdentity): string {
  return [
    CANVAS_PRESENTATION_PREFIX,
    canvasPresentationTabId(),
    identity.userId,
    identity.workspaceId,
    identity.taskId,
    identity.canvasId,
  ]
    .map(encodeURIComponent)
    .join(".");
}

/** Whether a task canvas was presented in this browser tab. */
export function wasCanvasPresented(identity: CanvasPresentationIdentity): boolean {
  const key = canvasPresentationKey(identity);
  const storage = sessionStorageOrNull();
  if (!storage) return memoryReceipts.has(key);

  try {
    return storage.getItem(key) === "1" || memoryReceipts.has(key);
  } catch {
    return memoryReceipts.has(key);
  }
}

/** Record a successful task canvas presentation in this browser tab. */
export function markCanvasPresented(identity: CanvasPresentationIdentity): void {
  const key = canvasPresentationKey(identity);
  const storage = sessionStorageOrNull();
  if (!storage) {
    memoryReceipts.add(key);
    return;
  }

  try {
    storage.setItem(key, "1");
  } catch {
    memoryReceipts.add(key);
  }
}

/** Shared seam for manual, restored, and automatic host presentation paths. */
export function recordCanvasPresentation(
  identity: CanvasPresentationIdentity,
  reason: CanvasPresentationReason,
): void {
  void reason;
  markCanvasPresented(identity);
}
