import {
  IconBox,
  IconBoxOff,
  IconCloud,
  IconCloudOff,
  IconFolder,
  IconFolders,
  IconPackage,
  IconPackageOff,
  IconServer,
  IconServerOff,
  IconTerminal2,
} from "@tabler/icons-react";

import { t } from "@/lib/i18n";

export const EXECUTOR_ICON_MAP: Record<string, typeof IconFolder> = {
  local: IconFolder,
  worktree: IconFolders,
  local_docker: IconBox,
  remote_docker: IconBox,
  sprites: IconCloud,
  ssh: IconTerminal2,
  k8s: IconPackage,
};

export function getExecutorIcon(type: string): typeof IconFolder {
  return EXECUTOR_ICON_MAP[type] ?? IconFolder;
}

// Brand and protocol names read the same in every locale, so they are values
// rather than copy — the guard's `words.exclude` lists both. Keeping them out
// of the catalog also keeps the pseudo-locale from transliterating them into
// something the user cannot match against their own SSH config or Sprites
// dashboard.
const EXECUTOR_BRAND_LABEL_MAP: Record<string, string> = {
  sprites: "Sprites.dev",
  ssh: "SSH",
};

// Keys, not messages: `t` resolves inside getExecutorLabel at call time, so
// the label follows a locale switch. Assigning t() here would freeze it at the
// boot locale.
const EXECUTOR_LABEL_KEY_MAP: Record<string, string> = {
  local: "executors:typeLocal",
  worktree: "executors:typeWorktree",
  local_docker: "executors:localDocker",
  remote_docker: "executors:remoteDocker",
  k8s: "executors:typeKubernetes",
};

export function getExecutorLabel(type: string): string {
  const brand = EXECUTOR_BRAND_LABEL_MAP[type];
  if (brand) return brand;
  const key = EXECUTOR_LABEL_KEY_MAP[type];
  // An unmapped type echoes the raw wire value, which is an identifier.
  return key ? t(key) : type;
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
  if (executorType === "ssh") {
    return {
      Icon: hasError ? IconServerOff : IconTerminal2,
      testId: "executor-status-ssh-icon",
    };
  }
  if (executorType === "k8s") {
    return {
      Icon: hasError ? IconPackageOff : IconPackage,
      testId: "executor-status-kubernetes-icon",
    };
  }
  return {
    Icon: hasError ? IconServerOff : IconServer,
    testId: "executor-status-server-icon",
  };
}
