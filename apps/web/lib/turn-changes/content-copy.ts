import { ApiError } from "@/lib/api/client";
import type { TurnChangeSetSummary, TurnFileChange } from "@/lib/types/turn-changes";

const CONTENT_REASON_KEYS: Record<string, string> = {
  export_timeout: "turnChangesContentReasonTimeout",
  export_missing_object: "turnChangesContentReasonMissing",
  export_interrupted: "turnChangesContentReasonInterrupted",
  size_limit: "turnChangesContentReasonSizeLimit",
};

export function contentUnavailableText(
  file: TurnFileChange | null,
  t: (key: string) => string,
): string {
  const key = CONTENT_REASON_KEYS[file?.content_reason ?? ""];
  return key ? t(`task:${key}`) : t("task:turnChangesContentUnavailable");
}

export function isSelectedContentPending(
  file: TurnFileChange | null,
  summary: TurnChangeSetSummary | undefined,
  hasPatch: boolean,
  contentError: unknown,
): boolean {
  if (!file || hasPatch) return false;
  if (contentError && !isContentNotReadyError(contentError)) return false;
  if (!file.synthetic && file.content_availability !== "pending") return false;
  const repository = summary?.repositories.find(
    (candidate) => candidate.id === file.repository_change_id,
  );
  if (!repository) return false;
  if (repository.export_state === "unavailable" || repository.export_state === "complete") {
    return false;
  }
  if (repository.content_complete) return false;
  // A synthetic placeholder has no canonical status. A missing content
  // response means the durable export has not reached it yet.
  return true;
}

function isContentNotReadyError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}
