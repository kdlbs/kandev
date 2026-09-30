import type { AutonomyRead, AutonomySpend } from "@/lib/api/domains/coordinator-autonomy-api";
import {
  isPersistentHoldReason,
  isStopFailing,
  serverNowMs,
  wireMs,
  type PersistentHoldReason,
} from "@/lib/coordinator/autonomy";

export type StripState =
  | { kind: "off" }
  | { kind: "active" }
  | { kind: "busy" }
  | { kind: "cooldown"; untilMs: number }
  | { kind: "held"; reason: PersistentHoldReason; detail: string }
  | { kind: "unknown" };

/** The strip's state from the admission block; anything unrecognized is `unknown`, never healthy. */
export function stripState(value: AutonomyRead): StripState {
  if (!value.autonomy_enabled) return { kind: "off" };
  const admission = value.admission;
  if (!admission) return { kind: "unknown" };
  if (admission.ok) return { kind: "active" };
  if (admission.reason === "conversation_busy") return { kind: "busy" };
  if (admission.reason === "cooldown") {
    const untilMs = wireMs(admission.until);
    return untilMs === null ? { kind: "unknown" } : { kind: "cooldown", untilMs };
  }
  if (isPersistentHoldReason(admission.reason)) {
    return { kind: "held", reason: admission.reason, detail: admission.detail };
  }
  return { kind: "unknown" };
}

/** Whether the strip renders at all for a usable read: autonomy on, or a stop that is failing. */
export function stripVisible(value: AutonomyRead): boolean {
  return value.autonomy_enabled || isStopFailing(value);
}

/** Milliseconds since the last wake on the server clock, or null when never woken or malformed. */
export function lastWokeAgeMs(value: AutonomyRead, loadedAt: number, now: number): number | null {
  const wokeMs = wireMs(value.last_woke_at);
  if (wokeMs === null) return null;
  return serverNowMs(value, loadedAt, now) - wokeMs;
}

export type SpendView =
  | { kind: "unmeasurable"; degraded: boolean }
  | { kind: "no-ceiling"; windowSubcents: number }
  | { kind: "ceiling"; windowSubcents: number; ceilingSubcents: number; over: boolean };

/** The spend rendering of the strip and settings: window at or above the ceiling is Over. */
export function spendView(spend: AutonomySpend): SpendView {
  if (!spend.measurable || spend.window_subcents === null) {
    return { kind: "unmeasurable", degraded: spend.degraded };
  }
  if (spend.ceiling_subcents === null) {
    return { kind: "no-ceiling", windowSubcents: spend.window_subcents };
  }
  return {
    kind: "ceiling",
    windowSubcents: spend.window_subcents,
    ceilingSubcents: spend.ceiling_subcents,
    over: spend.window_subcents >= spend.ceiling_subcents,
  };
}
