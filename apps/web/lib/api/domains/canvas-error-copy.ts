import { ApiError } from "../client";

type Translate = (key: string) => string;

const RUNTIME_TOKEN_EXPIRED_KEY = "canvases:runtimeTokenExpired";
const CANVAS_UNAVAILABLE_KEY = "canvases:unavailableDescription";

const CANVAS_API_ERROR_TRANSLATIONS: Record<string, string> = {
  runtime_token_expired: RUNTIME_TOKEN_EXPIRED_KEY,
  runtime_token_invalid: RUNTIME_TOKEN_EXPIRED_KEY,
  runtime_token_stale: RUNTIME_TOKEN_EXPIRED_KEY,
  canvas_not_found: CANVAS_UNAVAILABLE_KEY,
  pending_first_release: "canvases:pendingFirstReleaseDescription",
  permission_review_required: "canvases:pendingPermissionDescription",
  invalid_release: "canvases:invalidReleaseDescription",
  runtime_unavailable: CANVAS_UNAVAILABLE_KEY,
  canvas_release_unavailable: CANVAS_UNAVAILABLE_KEY,
  artifact_unavailable: CANVAS_UNAVAILABLE_KEY,
  active_release_missing: CANVAS_UNAVAILABLE_KEY,
  promotion_review_stale: "canvases:actionFailed",
  canvas_edit_stale: "canvases:actionFailed",
};

function apiErrorCode(error: ApiError): string | null {
  if (error.errorCode) return error.errorCode;
  if (!error.body || typeof error.body !== "object") return null;
  const code = (error.body as { error?: unknown }).error;
  return typeof code === "string" && code.trim() ? code : null;
}

export function canvasApiErrorTranslationKey(error: unknown): string | null {
  if (!(error instanceof ApiError)) return null;
  return CANVAS_API_ERROR_TRANSLATIONS[apiErrorCode(error) ?? ""] ?? null;
}

export function canvasErrorCodeMessage(
  code: string | undefined,
  translate: Translate,
  fallbackKey: string,
): string {
  const translationKey = CANVAS_API_ERROR_TRANSLATIONS[code?.trim() ?? ""];
  return translate(translationKey ?? fallbackKey);
}

/**
 * API errors are rendered from stable server codes. Other errors keep their
 * existing diagnostic text because they do not have a stable API contract.
 */
export function canvasErrorMessage(
  error: unknown,
  translate: Translate,
  fallbackKey: string,
): string {
  const translationKey = canvasApiErrorTranslationKey(error);
  if (translationKey) return translate(translationKey);
  if (error instanceof ApiError) return translate(fallbackKey);
  if (error instanceof Error && error.message.trim()) return error.message;
  return translate(fallbackKey);
}
