import { t } from "@/lib/i18n";

export type FileChangeStatus = "modified" | "added" | "deleted" | "untracked" | "renamed";

export function normalizeFileChangeStatus(status?: string): FileChangeStatus {
  switch (status) {
    case "added":
    case "modified":
    case "deleted":
    case "untracked":
    case "renamed":
      return status;
    case "removed":
      return "deleted";
    default:
      return "modified";
  }
}

/**
 * Accessible name and tooltip for a changed file's status marker.
 *
 * Resolves at call time (`FileStatusIcon` calls it from its render body), so a
 * locale switch re-labels the markers.
 */
export function fileChangeStatusLabel(status: FileChangeStatus, oldPath?: string): string {
  switch (status) {
    case "added":
      return t("diff:fileStatusAdded");
    case "untracked":
      return t("diff:fileStatusUntracked");
    case "modified":
      return t("diff:fileStatusModified");
    case "deleted":
      return t("diff:fileStatusDeleted");
    case "renamed":
      return oldPath ? t("diff:fileStatusMovedFrom", { oldPath }) : t("diff:fileStatusMoved");
    default: {
      const exhaustiveStatus: never = status;
      return exhaustiveStatus;
    }
  }
}
