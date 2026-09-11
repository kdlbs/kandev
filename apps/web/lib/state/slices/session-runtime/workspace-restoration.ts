import { generateUUID } from "@/lib/utils";

export type WorkspaceRestorationStatus = "pending" | "ready" | "error";

export type WorkspaceRestorationAttempt = {
  /** Stable identity that survives migration from a session fallback key. */
  attemptId?: string;
  taskId: string;
  sessionId: string;
  environmentId: string;
  revision: number;
  status: WorkspaceRestorationStatus;
  details?: string;
};

export type WorkspaceRestorationState = {
  byEnvironmentId: Record<string, WorkspaceRestorationAttempt>;
};

export type WorkspaceRestorationActions = {
  beginWorkspaceRestoration: (
    taskId: string,
    sessionId: string,
    environmentId: string,
  ) => WorkspaceRestorationAttempt | null;
  completeWorkspaceRestoration: (attempt: WorkspaceRestorationAttempt) => boolean;
  failWorkspaceRestoration: (attempt: WorkspaceRestorationAttempt, details: string) => boolean;
  clearWorkspaceRestoration: (attempt: WorkspaceRestorationAttempt) => boolean;
};

export type WorkspaceRestorationInput = {
  taskId: string;
  sessionId: string;
  environmentId: string;
};

export function resolveWorkspaceRestorationKey(
  _sessionId: string | null | undefined,
  environmentId?: string | null,
): string | null {
  const explicitEnvironmentId = environmentId?.trim();
  if (explicitEnvironmentId) return explicitEnvironmentId;
  // Workspace state is environment-scoped. Waiting for the canonical mapping
  // prevents a late mapping from moving an in-flight attempt away from the
  // key selected by a restore callback.
  return null;
}

export function beginWorkspaceRestoration(
  state: WorkspaceRestorationState,
  input: WorkspaceRestorationInput,
): WorkspaceRestorationAttempt | null {
  const current = state.byEnvironmentId[input.environmentId];
  if (
    current?.status === "pending" &&
    current.taskId === input.taskId &&
    current.sessionId === input.sessionId
  ) {
    return null;
  }
  const attempt: WorkspaceRestorationAttempt = {
    attemptId: generateUUID(),
    ...input,
    revision: (current?.revision ?? 0) + 1,
    status: "pending",
  };
  state.byEnvironmentId[input.environmentId] = attempt;
  return attempt;
}

function matchesAttemptFields(
  current: WorkspaceRestorationAttempt | undefined,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  if (!current) return false;
  if (
    attempt.attemptId !== undefined &&
    current.attemptId !== undefined &&
    current.attemptId !== attempt.attemptId
  ) {
    return false;
  }
  return (
    current.revision === attempt.revision &&
    current.taskId === attempt.taskId &&
    current.sessionId === attempt.sessionId
  );
}

function findMatchingWorkspaceRestorationAttempt(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): [key: string, current: WorkspaceRestorationAttempt] | null {
  const directMatch = state.byEnvironmentId[attempt.environmentId];
  if (matchesAttemptFields(directMatch, attempt)) {
    return [attempt.environmentId, directMatch];
  }

  for (const [key, current] of Object.entries(state.byEnvironmentId)) {
    if (matchesAttemptFields(current, attempt)) {
      return [key, current];
    }
  }
  return null;
}

export function isWorkspaceRestorationAttemptCurrent(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  return findMatchingWorkspaceRestorationAttempt(state, attempt) !== null;
}

export function completeWorkspaceRestoration(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  const match = findMatchingWorkspaceRestorationAttempt(state, attempt);
  if (!match) return false;
  const [key, current] = match;
  state.byEnvironmentId[key] = {
    ...current,
    status: "ready",
    details: undefined,
  };
  return true;
}

export function failWorkspaceRestoration(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
  details: string,
): boolean {
  const match = findMatchingWorkspaceRestorationAttempt(state, attempt);
  if (!match) return false;
  const [key, current] = match;
  state.byEnvironmentId[key] = {
    ...current,
    status: "error",
    details,
  };
  return true;
}

export function clearWorkspaceRestoration(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  const match = findMatchingWorkspaceRestorationAttempt(state, attempt);
  if (!match) return false;
  const [key] = match;
  delete state.byEnvironmentId[key];
  return true;
}

/** Keep backend diagnostics safe and bounded before they reach a disclosure. */
export function sanitizeWorkspaceRestorationDetails(error: unknown): string {
  let message = String(error ?? "");
  if (error instanceof Error) message = error.message;
  if (typeof error === "string") message = error;
  return message
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g, "")
    .slice(0, 512)
    .replace(/[\uD800-\uDBFF]$/u, "")
    .trim();
}
