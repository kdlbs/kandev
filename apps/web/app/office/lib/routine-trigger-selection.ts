import type { RoutineTrigger } from "@/lib/state/slices/office/types";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";

export type PrimaryCronTriggerSelection = {
  /** The AC-003.1 primary when one exists, otherwise the AC-003.3 fallback, otherwise undefined. */
  trigger: RoutineTrigger | undefined;
  /** True only when `trigger` is the AC-003.1 primary (has a resolvable future-or-past fire time). */
  isPrimary: boolean;
};

function byIdAscending(a: RoutineTrigger, b: RoutineTrigger): number {
  if (a.id < b.id) return -1;
  if (a.id > b.id) return 1;
  return 0;
}

/**
 * Selects the one cron trigger a routine's surfaces describe (REQ-003). The
 * primary is the enabled cron trigger with the earliest `nextRunAt` that
 * `parseTurnTimestamp` resolves to a value, tiebroken by `id`. With no
 * primary but at least one cron trigger, the fallback is the cron trigger
 * with the lowest `id`. With no cron trigger at all, neither exists.
 *
 * This is the one place that resolves "the" cron trigger; the list row, the
 * detail read-only card, the detail page's editable fields, and trigger sync
 * all call it so they can never disagree about which trigger they describe.
 */
export function selectPrimaryCronTrigger(triggers: RoutineTrigger[]): PrimaryCronTriggerSelection {
  const cronTriggers = triggers.filter((t) => t.kind === "cron");
  if (cronTriggers.length === 0) return { trigger: undefined, isPrimary: false };

  const candidates = cronTriggers
    .map((trigger) => ({
      trigger,
      ts: trigger.enabled ? parseTurnTimestamp(trigger.nextRunAt) : null,
    }))
    .filter((c): c is { trigger: RoutineTrigger; ts: bigint } => c.ts !== null);

  if (candidates.length > 0) {
    candidates.sort((a, b) => {
      if (a.ts < b.ts) return -1;
      if (a.ts > b.ts) return 1;
      return byIdAscending(a.trigger, b.trigger);
    });
    return { trigger: candidates[0].trigger, isPrimary: true };
  }

  const fallback = [...cronTriggers].sort(byIdAscending)[0];
  return { trigger: fallback, isPrimary: false };
}
