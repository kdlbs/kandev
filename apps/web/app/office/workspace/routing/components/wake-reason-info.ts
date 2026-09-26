import type { WakeReason } from "@/lib/state/slices/office/types";

// WakeReasonCopy describes one row in the wake-reason tier policy
// surface. Shared between the workspace settings card and the
// per-agent override panel so the explanation stays consistent.
export type WakeReasonCopy = {
  id: WakeReason;
  /** Catalog KEYS, not messages — see the note on `WAKE_REASONS`. */
  labelKey: string;
  shortKey: string;
  longKey: string;
  recommendationKey: string;
};

/**
 * Catalog keys, not copy: this table is module scope, so a `t()` here would
 * resolve once at import and freeze at the boot locale. Both consumers — the
 * workspace card and the per-agent override panel — resolve at render.
 *
 * `id` is the wire `WakeReason` value and is never translated.
 */
export const WAKE_REASONS: WakeReasonCopy[] = [
  {
    id: "heartbeat",
    labelKey: "office:wakeHeartbeat",
    shortKey: "office:wakeHeartbeatShort",
    longKey: "office:wakeHeartbeatLong",
    recommendationKey: "office:wakeHeartbeatRecommendation",
  },
  {
    id: "routine_trigger",
    labelKey: "office:wakeRoutineTrigger",
    shortKey: "office:wakeRoutineTriggerShort",
    longKey: "office:wakeRoutineTriggerLong",
    recommendationKey: "office:wakeRoutineTriggerRecommendation",
  },
  {
    id: "budget_alert",
    labelKey: "office:wakeBudgetAlert",
    shortKey: "office:wakeBudgetAlertShort",
    longKey: "office:wakeBudgetAlertLong",
    recommendationKey: "office:wakeBudgetAlertRecommendation",
  },
];

// USE_AGENT_TIER is the sentinel value used in the wake-reason tier
// dropdown to mean "no policy — use whatever tier this agent normally
// uses." Persisting this clears the key from the TierPerReason map.
export const USE_AGENT_TIER = "__inherit__" as const;
