import { t } from "@/lib/i18n";

type AvailabilityInput = {
  isLoading?: boolean;
  hasActiveTurn?: boolean;
};

/** True when any session attached to this task is mid-turn or tool call. */
export function hasActiveTaskSourceWork(
  taskSessionIds: readonly string[],
  activeTurnBySession: Readonly<Record<string, string | null | undefined>>,
): boolean {
  return taskSessionIds.some((sessionId) => Boolean(activeTurnBySession[sessionId]));
}

export function getAddSourcesDisabledReason({
  isLoading,
  hasActiveTurn,
}: AvailabilityInput): string | undefined {
  if (isLoading) return t("task:waitForTaskSourcesToLoad");
  if (hasActiveTurn) return t("task:waitForActiveTurnToFinish");
  return undefined;
}
