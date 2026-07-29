import { getSessionStorage, setSessionStorage, removeSessionStorage } from "@/lib/local-storage";

const STORAGE_PREFIX = "kandev.messageFavorites.";

/** Stores a session's favorite message IDs, removing empty sets. */
export function persistSessionFavorites(sessionId: string, messageIds: string[]): void {
  if (messageIds.length === 0) {
    removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
    return;
  }
  setSessionStorage(`${STORAGE_PREFIX}${sessionId}`, messageIds);
}

/** Loads the favorite message IDs previously stored for a session. */
export function loadSessionFavorites(sessionId: string): string[] {
  const value = getSessionStorage<string[]>(`${STORAGE_PREFIX}${sessionId}`, []);
  return Array.isArray(value) && value.every((id) => typeof id === "string") ? value : [];
}

/** Removes all persisted favorite message IDs for a session. */
export function clearPersistedSessionFavorites(sessionId: string): void {
  removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
}

/** Prefix used for session-scoped favorite message storage keys. */
export const MESSAGE_FAVORITES_STORAGE_PREFIX = STORAGE_PREFIX;
