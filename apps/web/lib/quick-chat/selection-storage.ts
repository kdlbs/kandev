import type {
  QuickChatSelection,
  QuickChatSelectionByWorkspace,
  QuickChatSelectionOrder,
  QuickChatSessionKind,
} from "@/lib/state/slices/ui/types";

export const ANONYMOUS_QUICK_CHAT_SELECTION_IDENTITY = "anonymous";
const STORAGE_PREFIX = "kandev.quick-chat.selection.v1.";
const STORAGE_VERSION = 1;
const MAX_WORKSPACES = 200;

export type QuickChatSelectionIdentity = string | null;

export type QuickChatSelectionStorageResult = {
  selections: QuickChatSelectionByWorkspace;
  order: QuickChatSelectionOrder;
};

type StoredSelectionEntry = {
  workspaceId: string;
  chat?: string;
  config?: string;
  lastSelectedAt?: number;
};

type StoredSelectionDocument = {
  version: number;
  entries: StoredSelectionEntry[];
};

type AuthIdentityState = {
  mode: "disabled" | "setup" | "enabled";
  authenticated: boolean;
  user: { id: string } | null;
};

function storageKey(identity: QuickChatSelectionIdentity): string | null {
  return identity === null ? null : `${STORAGE_PREFIX}${encodeURIComponent(identity)}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isSessionId(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function readEntry(value: unknown): StoredSelectionEntry | null {
  if (!isRecord(value) || typeof value.workspaceId !== "string" || !value.workspaceId) {
    return null;
  }
  const entry: StoredSelectionEntry = { workspaceId: value.workspaceId };
  for (const kind of ["chat", "config"] as const) {
    if (isSessionId(value[kind])) entry[kind] = value[kind];
  }
  if (typeof value.lastSelectedAt === "number" && Number.isFinite(value.lastSelectedAt)) {
    entry.lastSelectedAt = value.lastSelectedAt;
  }
  return entry.chat || entry.config ? entry : null;
}

function emptySelection(): QuickChatSelectionStorageResult {
  return { selections: {}, order: [] };
}

/** Returns the browser-storage identity for the current authentication mode. */
export function getQuickChatSelectionIdentity(auth: AuthIdentityState): QuickChatSelectionIdentity {
  if (auth.mode === "disabled") return ANONYMOUS_QUICK_CHAT_SELECTION_IDENTITY;
  if (auth.mode !== "enabled" || !auth.authenticated || !auth.user?.id) return null;
  return auth.user.id;
}

/** Reads and validates remembered Quick Chat selections for one identity. */
export function loadQuickChatSelection(
  identity: QuickChatSelectionIdentity,
): QuickChatSelectionStorageResult {
  const key = storageKey(identity);
  if (!key || typeof window === "undefined") return emptySelection();

  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return emptySelection();
    const parsed: unknown = JSON.parse(raw);
    if (!isRecord(parsed) || parsed.version !== STORAGE_VERSION || !Array.isArray(parsed.entries)) {
      return emptySelection();
    }

    const entries = parsed.entries
      .map(readEntry)
      .filter((entry): entry is StoredSelectionEntry => entry !== null)
      .map((entry, index) => ({ entry, index }))
      .sort(
        (left, right) =>
          (right.entry.lastSelectedAt ?? 0) - (left.entry.lastSelectedAt ?? 0) ||
          left.index - right.index,
      );
    const selections: QuickChatSelectionByWorkspace = {};
    const order: QuickChatSelectionOrder = [];
    for (const { entry } of entries) {
      if (order.length >= MAX_WORKSPACES || selections[entry.workspaceId]) continue;
      const selection: QuickChatSelection = {};
      if (entry.chat) selection.chat = entry.chat;
      if (entry.config) selection.config = entry.config;
      selections[entry.workspaceId] = selection;
      order.push(entry.workspaceId);
    }
    return { selections, order };
  } catch {
    return emptySelection();
  }
}

/** Writes the bounded remembered-selection map. Storage failures are harmless. */
export function persistQuickChatSelection(
  identity: QuickChatSelectionIdentity,
  selections: QuickChatSelectionByWorkspace,
  order: QuickChatSelectionOrder,
): void {
  const key = storageKey(identity);
  if (!key || typeof window === "undefined") return;

  const orderedWorkspaceIds = [
    ...order,
    ...Object.keys(selections).filter((workspaceId) => !order.includes(workspaceId)),
  ].filter((workspaceId, index, all) => all.indexOf(workspaceId) === index);
  const entries: StoredSelectionEntry[] = orderedWorkspaceIds
    .filter((workspaceId) => selections[workspaceId])
    .slice(0, MAX_WORKSPACES)
    .map((workspaceId, index) => {
      const selection = selections[workspaceId];
      return {
        workspaceId,
        ...(selection.chat ? { chat: selection.chat } : {}),
        ...(selection.config ? { config: selection.config } : {}),
        lastSelectedAt: Date.now() - index,
      };
    });

  try {
    window.localStorage.setItem(
      key,
      JSON.stringify({ version: STORAGE_VERSION, entries } satisfies StoredSelectionDocument),
    );
  } catch {
    // Browser storage can be disabled or full. The in-memory selection remains valid.
  }
}

export const QUICK_CHAT_SELECTION_MAX_WORKSPACES = MAX_WORKSPACES;
export type { QuickChatSessionKind };
