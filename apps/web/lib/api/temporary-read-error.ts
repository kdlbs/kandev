import { ApiError } from "@/lib/api/client";
import { TaskNavigationCycleDeadlineError } from "@/lib/state/task-navigation-recovery-clock";

/** One attempt exceeded its own wall-time deadline while the cycle still had budget. */
export class ReadAttemptDeadlineError extends Error {
  constructor() {
    super("Interactive read attempt deadline exceeded");
    this.name = "ReadAttemptDeadlineError";
  }
}

const TEMPORARY_STATUSES = new Set([429, 502, 503, 504]);

function isPersistenceUnavailable(body: unknown): boolean {
  return (
    body !== null &&
    typeof body === "object" &&
    "code" in body &&
    (body as { code?: unknown }).code === "persistence_unavailable"
  );
}

/**
 * Neutral classification for interactive reads. Network failures, temporary
 * availability responses, attempt deadlines, and the persistence admission gate
 * are recoverable; aborts, validation, authorization, and missing resources are not.
 */
export function isTemporaryReadError(error: unknown): boolean {
  if (error instanceof TypeError) return true;
  if (error instanceof TaskNavigationCycleDeadlineError) return true;
  if (error instanceof ReadAttemptDeadlineError) return true;
  if (!(error instanceof ApiError)) return false;
  if (TEMPORARY_STATUSES.has(error.status)) return true;
  return isPersistenceUnavailable(error.body);
}

/** Server-requested delay, never shortened by a recovery owner. */
export function readRetryAfterMs(error: unknown): number {
  return error instanceof ApiError ? (error.retryAfterSeconds ?? 0) * 1_000 : 0;
}

export function isReadAbort(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}
