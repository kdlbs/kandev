import { ApiError } from "@/lib/api/client";

export type PayloadRetentionMarker = { version: number; removed_at: string };

export function payloadRetentionMarker(metadata: unknown): PayloadRetentionMarker | null {
  if (!metadata || typeof metadata !== "object" || !("payload_retention" in metadata)) return null;
  const marker = metadata.payload_retention;
  if (!marker || typeof marker !== "object" || !("version" in marker) || !("removed_at" in marker))
    return null;
  return typeof marker.version === "number" && typeof marker.removed_at === "string"
    ? (marker as PayloadRetentionMarker)
    : null;
}

export function isToolPayloadRemovedError(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 410 &&
    typeof error.body === "object" &&
    error.body !== null &&
    "code" in error.body &&
    error.body.code === "tool_payload_removed"
  );
}
