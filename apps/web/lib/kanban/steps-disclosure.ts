export type DisclosureOverrides = Record<string, boolean>;

/**
 * A group defaults to expanded IFF its hidden set contains at least one id
 * that still matches a step in the workflow's current snapshot — the
 * discoverability guarantee a hidden step must never be silently un-reachable.
 */
export function defaultGroupExpanded(hiddenIds: string[], liveStepIds: Set<string>): boolean {
  return hiddenIds.some((id) => liveStepIds.has(id));
}

/** `overrides[workflowId] ?? defaultExpanded(workflowId)` — the disclosure resolution rule. */
export function effectiveGroupExpanded(
  workflowId: string,
  overrides: DisclosureOverrides,
  defaultValue: boolean,
): boolean {
  return overrides[workflowId] ?? defaultValue;
}

/**
 * Each explicit toggle writes the negation of the group's current effective
 * disclosure — never a fixed end value — so two toggles on a hidden-bearing
 * (expanded-default) group leave `true`, not `false`.
 */
export function toggleGroupDisclosure(
  workflowId: string,
  overrides: DisclosureOverrides,
  defaultValue: boolean,
): DisclosureOverrides {
  const current = effectiveGroupExpanded(workflowId, overrides, defaultValue);
  return { ...overrides, [workflowId]: !current };
}

/**
 * The shown-count summary ("N of M shown") counts only live steps: a stale
 * hidden id (one matching no step in the current snapshot) is excluded from
 * both the numerator and the denominator, consistent with `H ∩ liveStepIds`.
 */
export function shownStepCount(
  liveStepIds: string[],
  hiddenIds: string[] | undefined,
): { shown: number; total: number } {
  const hidden = new Set(hiddenIds ?? []);
  const total = liveStepIds.length;
  const hiddenLiveCount = liveStepIds.filter((id) => hidden.has(id)).length;
  return { shown: total - hiddenLiveCount, total };
}
