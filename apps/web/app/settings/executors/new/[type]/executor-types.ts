// Registry of executor types presented in the "new executor profile" flow.
// Keep entries here (not inline in the page) so the page file stays under
// the 600-line lint cap and new types can be added without touching layout.
//
// The map keys (`local`, `worktree`, `local_docker`, `remote_docker`,
// `sprites`, `ssh`, `k8s`) are the persisted executor enum and the create route's
// path segment, and `executorId` is a backend row id — none of them is copy.
// Labels and descriptions travel as catalog keys and resolve at render;
// `brandLabel` carries a brand/protocol name that reads the same in every
// locale, so it is a value rather than a message.

export type ExecutorTypeInfo = {
  executorId: string;
  /** Rendered verbatim. Set on brand/protocol names only. */
  brandLabel?: string;
  /** Catalog key for the label. Set when `brandLabel` is not. */
  labelKey?: string;
  descriptionKey: string;
};

export const EXECUTOR_TYPE_MAP: Record<string, ExecutorTypeInfo> = {
  local: {
    executorId: "exec-local",
    labelKey: "executors:typeLocal",
    descriptionKey: "executors:descriptionLocalPc",
  },
  worktree: {
    executorId: "exec-worktree",
    labelKey: "executors:typeWorktree",
    descriptionKey: "executors:descriptionWorktree",
  },
  local_docker: {
    executorId: "exec-local-docker",
    brandLabel: "Docker",
    descriptionKey: "executors:descriptionLocalDocker",
  },
  remote_docker: {
    executorId: "exec-remote-docker",
    labelKey: "executors:remoteDocker",
    descriptionKey: "executors:descriptionRemoteDocker",
  },
  sprites: {
    executorId: "exec-sprites",
    brandLabel: "Sprites.dev",
    descriptionKey: "executors:descriptionSprites",
  },
  // `CreateProfilePage` short-circuits `ssh` to `SSHCreatePage`, which renders
  // its own header — this entry exists so the unknown-type guard accepts the
  // route, and its label/description are never rendered.
  ssh: {
    executorId: "exec-ssh",
    brandLabel: "SSH",
    descriptionKey: "executors:descriptionSshCreate",
  },
  k8s: {
    executorId: "exec-k8s",
    labelKey: "executors:typeKubernetes",
    descriptionKey: "executors:descriptionKubernetes",
  },
};

/** The label for a type card: a brand name verbatim, or the translated label. */
export function executorTypeLabel(info: ExecutorTypeInfo, t: (key: string) => string): string {
  return info.brandLabel ?? (info.labelKey ? t(info.labelKey) : "");
}
