import { t } from "@/lib/i18n";
import type { ExecutorType } from "@/lib/types/http";

/**
 * Resolves at call time rather than module load, so the description follows a
 * locale switch. The five non-SSH sentences are byte-identical to the ones the
 * `/settings/executor/:id` route already renders, so they share its keys;
 * `descriptionSsh` is this surface's own sentence and deliberately differs from
 * the one `app/settings/executors/new/[type]/executor-types.ts` shows on the
 * create page — see the guard comment in eslint.i18n.options.mjs.
 */
export function getExecutorDescription(type: ExecutorType): string {
  if (type === "local_pc") return t("executors:descriptionLocalPc");
  if (type === "worktree") return t("executors:descriptionWorktree");
  if (type === "local_docker") return t("executors:descriptionLocalDocker");
  if (type === "remote_docker") return t("executors:descriptionRemoteDocker");
  if (type === "sprites") return t("executors:descriptionSprites");
  if (type === "ssh") return t("executors:descriptionSsh");
  return t("executors:descriptionCustom");
}
