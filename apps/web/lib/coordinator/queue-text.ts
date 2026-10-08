import type { TFunction } from "i18next";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";

/**
 * The Working row's agent-state text: every Working item's primary session
 * state is RUNNING or STARTING, since that is how it was classified into the
 * group (AC-COORDINATOR-NEEDS-YOU-004.3).
 */
export function agentStateLabel(t: TFunction, state: string | undefined): string {
  return state === "RUNNING"
    ? t("coordinator:agentStateRunning")
    : t("coordinator:agentStateStarting");
}

/**
 * The Other group's exception text for a task whose position could not be
 * derived from its session, or that has no session at all
 * (AC-COORDINATOR-NEEDS-YOU-004.3). Undefined when neither exception
 * applies, in which case the row shows no extra status column.
 */
export function otherRowStatusText(
  t: TFunction,
  item: QueueItem,
  task: AttentionTask,
): string | undefined {
  if (item.group !== "other") return undefined;
  if (item.sessionUnreadable) return t("coordinator:positionUnderivable");
  if (task.statusSummary && task.statusSummary.primary_session === null) {
    return t("coordinator:noSession");
  }
  return undefined;
}
