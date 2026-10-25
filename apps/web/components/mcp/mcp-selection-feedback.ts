import { ApiError, isHandledApiError } from "@/lib/api/client";

export function mcpSelectionErrorKey(error: unknown) {
  if (!error || isHandledApiError(error)) return null;
  if (error instanceof ApiError && error.status === 403 && error.errorCode === "forbidden") {
    return "settings:mcpSelectionForbidden";
  }
  return "settings:mcpSelectionSaveFailed";
}
