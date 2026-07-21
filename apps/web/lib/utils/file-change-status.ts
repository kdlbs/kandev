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

export function fileChangeStatusLabel(status: FileChangeStatus, oldPath?: string): string {
  switch (status) {
    case "added":
      return "Added";
    case "untracked":
      return "Untracked";
    case "modified":
      return "Modified";
    case "deleted":
      return "Deleted";
    case "renamed":
      return oldPath ? `Moved from ${oldPath}` : "Moved";
    default: {
      const exhaustiveStatus: never = status;
      return exhaustiveStatus;
    }
  }
}
