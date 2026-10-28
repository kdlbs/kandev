import { ApiError, isHandledApiError } from "@/lib/api/client";

export function mcpSelectionErrorKey(error: unknown) {
  if (!error || isHandledApiError(error)) return null;
  if (error instanceof ApiError && error.status === 403 && error.errorCode === "forbidden") {
    return "settings:mcpSelectionForbidden";
  }
  if (
    error instanceof ApiError &&
    error.status === 409 &&
    typeof error.body === "object" &&
    error.body !== null &&
    "code" in error.body &&
    error.body.code === "mcp_selection_conflict"
  ) {
    return "settings:mcpSelectionConflict";
  }
  return "settings:mcpSelectionSaveFailed";
}
