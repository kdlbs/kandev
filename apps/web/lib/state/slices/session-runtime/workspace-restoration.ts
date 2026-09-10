export type WorkspaceRestorationStatus = "pending" | "ready" | "error";

export type WorkspaceRestorationAttempt = {
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
    ...input,
    revision: (current?.revision ?? 0) + 1,
    status: "pending",
  };
  state.byEnvironmentId[input.environmentId] = attempt;
  return attempt;
}

function matchesWorkspaceRestorationAttempt(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  const current = state.byEnvironmentId[attempt.environmentId];
  return Boolean(
    current &&
    current.revision === attempt.revision &&
    current.taskId === attempt.taskId &&
    current.sessionId === attempt.sessionId,
  );
}

export function completeWorkspaceRestoration(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  if (!matchesWorkspaceRestorationAttempt(state, attempt)) return false;
  state.byEnvironmentId[attempt.environmentId] = {
    ...attempt,
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
  if (!matchesWorkspaceRestorationAttempt(state, attempt)) return false;
  state.byEnvironmentId[attempt.environmentId] = {
    ...attempt,
    status: "error",
    details,
  };
  return true;
}

export function clearWorkspaceRestoration(
  state: WorkspaceRestorationState,
  attempt: WorkspaceRestorationAttempt,
): boolean {
  if (!matchesWorkspaceRestorationAttempt(state, attempt)) return false;
  delete state.byEnvironmentId[attempt.environmentId];
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
