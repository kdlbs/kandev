import { IconFolder, IconFolders, IconBrandDocker, IconCloud } from "@tabler/icons-react";

export const EXECUTOR_ICON_MAP: Record<string, typeof IconFolder> = {
  local: IconFolder,
  worktree: IconFolders,
  local_docker: IconBrandDocker,
  remote_docker: IconCloud,
};

export function getExecutorIcon(type: string): typeof IconFolder {
  return EXECUTOR_ICON_MAP[type] ?? IconFolder;
}

const EXECUTOR_LABEL_MAP: Record<string, string> = {
  local: "Local",
  worktree: "Worktree",
  local_docker: "Local Docker",
  remote_docker: "Remote Docker",
};

export function getExecutorLabel(type: string): string {
  return EXECUTOR_LABEL_MAP[type] ?? type;
}
