import { getSessionStorage, setSessionStorage, removeSessionStorage } from "@/lib/local-storage";
import type { Comment } from "./types";

const STORAGE_PREFIX = "kandev.comments.";

export function persistSessionComments(sessionId: string, comments: Comment[]): void {
  if (comments.length === 0) {
    removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
    return;
  }
  setSessionStorage(`${STORAGE_PREFIX}${sessionId}`, JSON.parse(JSON.stringify(comments)));
}

export function loadSessionComments(sessionId: string): Comment[] {
  return getSessionStorage(`${STORAGE_PREFIX}${sessionId}`, [] as Comment[]) as Comment[];
}

export function clearPersistedSessionComments(sessionId: string): void {
  removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
}

export const COMMENTS_STORAGE_PREFIX = STORAGE_PREFIX;
