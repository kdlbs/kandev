import type { WorkspaceContextReadError } from "./slices/kanban/types";

export type WorkspaceContextState = {
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
};

export function isCurrentWorkspaceContext(
  state: WorkspaceContextState,
  requestedWorkspaceId: string | null,
  requestedGeneration: number,
): boolean {
  return (
    state.workspaces.activeId === requestedWorkspaceId &&
    state.workspaceContextGeneration === requestedGeneration
  );
}

export function classifyWorkspaceContextReadError(error: unknown): WorkspaceContextReadError {
  if (isAbortError(error)) return "cancelled";
  const status = apiErrorStatus(error);
  if (status === undefined) return "transient";
  if (status === 401 || status === 403) return "access_denied";
  if (status === 404) return "not_found";
  if (status === 400 || status === 422) return "invalid";
  if ([429, 502, 503, 504].includes(status)) return "transient";
  return "unknown";
}

export function isRetryableWorkspaceContextError(error: unknown): boolean {
  return classifyWorkspaceContextReadError(error) === "transient";
}

export function retryAfterMilliseconds(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const retryAfterSeconds = (error as { retryAfterSeconds?: unknown }).retryAfterSeconds;
  return typeof retryAfterSeconds === "number" && retryAfterSeconds > 0
    ? retryAfterSeconds * 1000
    : undefined;
}

function apiErrorStatus(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const status = (error as { status?: unknown }).status;
  return typeof status === "number" ? status : undefined;
}

function isAbortError(error: unknown): boolean {
  return (
    (typeof DOMException !== "undefined" &&
      error instanceof DOMException &&
      error.name === "AbortError") ||
    (error instanceof Error && error.name === "AbortError")
  );
}
