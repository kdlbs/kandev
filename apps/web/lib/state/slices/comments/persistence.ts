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

function rawSessionComments(sessionId: string): unknown[] {
  const stored = getSessionStorage(`${STORAGE_PREFIX}${sessionId}`, [] as Comment[]) as unknown;
  return Array.isArray(stored) ? stored : [];
}

/** Discover only valid pending legacy plan rows for known sessions. */
export function listLegacyPlanComments(sessionIds: string[]): LegacyPlanCommentRecord[] {
  const result: LegacyPlanCommentRecord[] = [];
  for (const sessionId of sessionIds) {
    for (const value of rawSessionComments(sessionId)) {
      if (isLegacyPlanComment(value)) result.push({ sessionId, comment: value });
    }
  }
  return result;
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
  const index = values.findIndex((value) => sameLegacyPlanComment(value, expected));
  if (index < 0) return false;
  values.splice(index, 1);
  if (values.length === 0) removeSessionStorage(`${STORAGE_PREFIX}${sessionId}`);
  else setSessionStorage(`${STORAGE_PREFIX}${sessionId}`, values as Comment[]);
  return true;
}

export const COMMENTS_STORAGE_PREFIX = STORAGE_PREFIX;
