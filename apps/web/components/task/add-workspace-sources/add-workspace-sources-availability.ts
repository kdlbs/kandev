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
  if (isLoading) return "Wait for task sources to finish loading before adding sources.";
  if (hasActiveTurn)
    return "Wait for the active turn or tool call to finish before adding sources.";
  return undefined;
}
