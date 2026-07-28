import { getSessionStorage, setSessionStorage, removeSessionStorage } from "@/lib/local-storage";

const STORAGE_PREFIX = "kandev.messageFavorites.";

export function persistSessionFavorites(sessionId: string, messageIds: string[]): void {
  if (messageIds.length === 0) {
    removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
    return;
  }
  setSessionStorage(`${STORAGE_PREFIX}${sessionId}`, messageIds);
}

export function loadSessionFavorites(sessionId: string): string[] {
  return getSessionStorage<string[]>(`${STORAGE_PREFIX}${sessionId}`, []);
}

export function clearPersistedSessionFavorites(sessionId: string): void {
  removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
}

export const MESSAGE_FAVORITES_STORAGE_PREFIX = STORAGE_PREFIX;
