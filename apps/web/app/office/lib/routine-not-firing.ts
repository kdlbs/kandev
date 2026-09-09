import { ApiError } from "@/lib/api/client";

/**
 * Stable machine-readable code the backend returns when a manual or
 * webhook fire is refused because the routine's status does not permit it
 * (apps/backend/internal/office/routines/status_gate.go). The frontend
 * selects localized copy from this code rather than the server's
 * human-readable string, which cannot be translated.
 */
export const ROUTINE_NOT_FIRING_ERROR_CODE = "routine_not_firing";

export function isRoutineNotFiringError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.errorCode === ROUTINE_NOT_FIRING_ERROR_CODE;
}

type Translate = (key: string, options?: Record<string, unknown>) => string;

function refusedStatus(error: ApiError): string {
  if (!error.body || typeof error.body !== "object") return "";
  const status = (error.body as { status?: unknown }).status;
  return typeof status === "string" ? status : "";
}

/**
 * Resolves the "Run now" failure toast. A status refusal renders localized
 * copy naming the observed status, interpolated from the response body's
 * `status` field rather than parsed out of the server's English sentence
 * (AC-OFFICE-ROUTINE-STATUS-006.4, -006.6). Any other error keeps the
 * existing behavior: the server message when there is one, else the
 * caller's fallback key.
 */
export function routineNotFiringMessage(error: unknown, t: Translate, fallbackKey: string): string {
  if (isRoutineNotFiringError(error)) {
    return t("office:routineNotFiring", { status: refusedStatus(error) });
  }
  if (error instanceof Error && error.message.trim()) return error.message;
  return t(fallbackKey);
}
