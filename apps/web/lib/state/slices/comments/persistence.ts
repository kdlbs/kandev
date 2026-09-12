import { getSessionStorage, setSessionStorage, removeSessionStorage } from "@/lib/local-storage";
import type { Comment, PlanComment } from "./types";

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

export type LegacyPlanCommentRecord = { sessionId: string; comment: PlanComment };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isLegacyPlanComment(value: unknown): value is PlanComment {
  if (!isRecord(value)) return false;
  return (
    value.source === "plan" &&
    value.status === "pending" &&
    typeof value.id === "string" &&
    typeof value.sessionId === "string" &&
    typeof value.text === "string" &&
    typeof value.selectedText === "string" &&
    typeof value.createdAt === "string" &&
    (value.from === undefined || typeof value.from === "number") &&
    (value.to === undefined || typeof value.to === "number")
  );
}

function rawSessionComments(sessionId: string): unknown[] | null {
  try {
    const raw = window.sessionStorage.getItem(`${STORAGE_PREFIX}${sessionId}`);
    const stored: unknown = raw === null ? [] : JSON.parse(raw);
    return Array.isArray(stored) ? stored : null;
  } catch {
    return null;
  }
}

/** Discover only valid pending legacy plan rows for known sessions. */
export function listLegacyPlanComments(sessionIds: string[]): LegacyPlanCommentRecord[] {
  return readLegacyPlanComments(sessionIds).records;
}

/** An unavailable scan cannot prove that previously identified drafts disappeared. */
export function readLegacyPlanComments(sessionIds: string[]) {
  const records: LegacyPlanCommentRecord[] = [];
  let available = true;
  for (const sessionId of sessionIds) {
    const values = rawSessionComments(sessionId);
    if (values === null) available = false;
    for (const value of values ?? []) {
      if (isLegacyPlanComment(value)) records.push({ sessionId, comment: value });
    }
  }
  return { records, available };
}

function sameLegacyPlanComment(value: unknown, expected: PlanComment): boolean {
  if (!isLegacyPlanComment(value)) return false;
  return (
    value.id === expected.id &&
    value.sessionId === expected.sessionId &&
    value.text === expected.text &&
    value.selectedText === expected.selectedText &&
    value.from === expected.from &&
    value.to === expected.to &&
    value.createdAt === expected.createdAt &&
    value.status === expected.status
  );
}

/** Reread storage and remove only the exact row whose upload was acknowledged. */
export function removeAcknowledgedLegacyPlanComment(
  sessionId: string,
  expected: PlanComment,
): boolean {
  const values = rawSessionComments(sessionId);
  if (values === null) return false;
  const index = values.findIndex((value) => sameLegacyPlanComment(value, expected));
  if (index < 0) return !values.some((value) => isRecord(value) && value.id === expected.id);
  try {
    values.splice(index, 1);
    if (values.length === 0) window.sessionStorage.removeItem(`${STORAGE_PREFIX}${sessionId}`);
    else window.sessionStorage.setItem(`${STORAGE_PREFIX}${sessionId}`, JSON.stringify(values));
    const remaining = rawSessionComments(sessionId);
    return (
      remaining !== null && !remaining.some((value) => isRecord(value) && value.id === expected.id)
    );
  } catch {
    return false;
  }
}

export const COMMENTS_STORAGE_PREFIX = STORAGE_PREFIX;
