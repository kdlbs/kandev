import { t } from "@/lib/i18n";

const MULTI_REPO_SUPPORTED_EXECUTOR_TYPES = new Set([
  "worktree",
  "local_docker",
  "ssh",
  "sprites",
  "k8s",
]);

/**
 * Returns the selector explanation for runtimes that cannot launch a task
 * with sibling repositories. This is deliberately a pure capability check:
 * changing repository rows must never replace a user's supported executor.
 *
 * The three messages are copy: they render as the `title` on a disabled option
 * in the automation editor's Executor Profile picker and in the task-create
 * dialog. The Automations migration translated all three but left the file off
 * `i18nGuardFiles`, arguing that allowlisting it would claim a completeness the
 * task-create dialog has not had. That reasoning was wrong, and the entry was
 * added here: the completeness the allowlist tracks is the FILE's, not the
 * consuming directory's, and this file holds no other copy. Left off, it could
 * silently drift back — which is the one thing the list exists to prevent.
 * Resolved at call time rather than module scope, so a locale switch is picked up.
 */
export function getMultiRepoExecutorDisabledReason(executorType: string | null | undefined) {
  if (MULTI_REPO_SUPPORTED_EXECUTOR_TYPES.has(executorType ?? "")) return null;
  if (executorType === "local" || executorType === "local_pc") {
    return t("common:multiRepoUnavailableLocal");
  }
  if (executorType === "remote_docker") {
    return t("common:multiRepoUnavailableRemoteDocker");
  }
  return t("common:multiRepoUnsupportedExecutor");
}
