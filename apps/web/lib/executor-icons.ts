import {
  IconBox,
  IconBoxOff,
  IconCloud,
  IconCloudOff,
  IconFolder,
  IconFolders,
  IconServer,
  IconServerOff,
} from "@tabler/icons-react";

export const EXECUTOR_ICON_MAP: Record<string, typeof IconFolder> = {
  local: IconFolder,
  worktree: IconFolders,
  local_docker: IconBox,
  remote_docker: IconBox,
  sprites: IconCloud,
};

export function getExecutorIcon(type: string): typeof IconFolder {
  return EXECUTOR_ICON_MAP[type] ?? IconFolder;
}

const EXECUTOR_LABEL_MAP: Record<string, string> = {
  local: "Local",
  worktree: "Worktree",
  local_docker: "Local Docker",
  remote_docker: "Remote Docker",
  sprites: "Sprites.dev",
};

export function getExecutorLabel(type: string): string {
  return EXECUTOR_LABEL_MAP[type] ?? type;
}

/**
 * Picks the status icon for the right-side executor popover button and the
 * left-side cloud tooltip on cards/lists. The "Off" variants signal an error
 * state (e.g. missing sandbox upstream) so the surface can swap glyph + color
 * without each caller inventing its own mapping.
 */
export function getExecutorStatusIcon(
  executorType: string | null | undefined,
  hasError: boolean,
): { Icon: typeof IconFolder; testId: string } {
  if (executorType === "local_docker" || executorType === "remote_docker") {
    return {
      Icon: hasError ? IconBoxOff : IconBox,
      testId: "executor-status-container-icon",
    };
  }
  if (executorType === "sprites") {
    return {
      Icon: hasError ? IconCloudOff : IconCloud,
      testId: "executor-status-cloud-icon",
    };
  }
  return {
    Icon: hasError ? IconServerOff : IconServer,
    testId: "executor-status-server-icon",
  };
}
