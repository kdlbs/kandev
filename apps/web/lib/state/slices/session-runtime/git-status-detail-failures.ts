import type { FileChangeFacet, FileInfo, GitStatusEntry } from "./types";

function hasRecoverableUnavailableDiff(
  detail: Pick<FileChangeFacet, "diff_state" | "diff_skip_reason"> | undefined,
): boolean {
  return detail?.diff_state === "unavailable" && !detail.diff_skip_reason;
}

function fileHasRecoverableUnavailableDiff(file: FileInfo): boolean {
  return (
    hasRecoverableUnavailableDiff(file) ||
    hasRecoverableUnavailableDiff(file.staged_change) ||
    hasRecoverableUnavailableDiff(file.unstaged_change)
  );
}

/** Whether complete Git membership contains detail failures that automatic recovery retries. */
export function hasRecoverableGitStatusDetailFailure(
  status: Pick<GitStatusEntry, "detail_state" | "files">,
): boolean {
  if (status.detail_state === "unavailable") return true;
  return Object.values(status.files ?? {}).some(fileHasRecoverableUnavailableDiff);
}
