import type { AutonomyRead, TurnRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { formatNumber } from "@/lib/i18n/formats";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

const NANOS_PER_MS = BigInt(1_000_000);
const STOP_GRACE_MS = 5 * 60_000;
const DEADLINE_PAD_MS = 1_000;

export const PERSISTENT_HOLD_REASONS = [
  "containment",
  "spend_unmeasured",
  "ceiling_reached",
  "no_conversation",
  "conversation_unavailable",
] as const;
export type PersistentHoldReason = (typeof PERSISTENT_HOLD_REASONS)[number];

export function isPersistentHoldReason(reason: string | undefined): reason is PersistentHoldReason {
  return !!reason && (PERSISTENT_HOLD_REASONS as readonly string[]).includes(reason);
}

/** Epoch ms of a strict RFC3339 wire timestamp, or null when missing or malformed. */
export function wireMs(value: string | null | undefined): number | null {
  const ns = parseStrictRfc3339Timestamp(value ?? undefined);
  return ns === null ? null : Number(ns / NANOS_PER_MS);
}

/** A read is usable only with the fields every consumer dereferences; anything else fails closed. */
export function isUsableAutonomy(value: AutonomyRead | null | undefined): value is AutonomyRead {
  if (!value || wireMs(value.server_time) === null) return false;
  if (typeof value.autonomy_enabled !== "boolean") return false;
  if (!Number.isInteger(value.pending_wakes) || value.pending_wakes < 0) return false;
  if (!value.spend || typeof value.spend.measurable !== "boolean") return false;
  return Array.isArray(value.containment?.conditions);
}

/** The server's clock now: `server_time` plus the time elapsed since the read landed. */
export function serverNowMs(value: AutonomyRead, loadedAt: number, now: number): number {
  return (wireMs(value.server_time) ?? now) + Math.max(0, now - loadedAt);
}

export function isTurnOpen(turn: TurnRead | null): turn is TurnRead {
  return !!turn && turn.finished_at === null && turn.outcome === null;
}

export function isStopFailing(value: AutonomyRead): boolean {
  return isTurnOpen(value.last_turn) && value.last_turn.stop_state === "stop_failing";
}

/**
 * The earliest server-clock instant at which a re-read changes what the strip
 * shows: a stop request turning stale, or a cooldown ending.
 */
export function nextDeadlineMs(value: AutonomyRead): number | null {
  const candidates: number[] = [];
  const turn = value.last_turn;
  if (isTurnOpen(turn) && turn.stop_state === null) {
    const requested = wireMs(turn.stop_requested_at);
    if (requested !== null) candidates.push(requested + STOP_GRACE_MS + DEADLINE_PAD_MS);
  }
  if (value.admission?.reason === "cooldown") {
    const until = wireMs(value.admission.until);
    if (until !== null) candidates.push(until + DEADLINE_PAD_MS);
  }
  return candidates.length ? Math.min(...candidates) : null;
}

const CEILING_PATTERN = /^\d{1,5}(\.\d{1,2})?$/;
const MIN_SUBCENTS = 1;
const MAX_SUBCENTS = 1_000_000;

/** A dollar string as integer cents, or null when it is not a valid 0.01 to 10000.00 ceiling. */
export function ceilingCents(text: string): number | null {
  const trimmed = text.trim();
  if (!CEILING_PATTERN.test(trimmed)) return null;
  const [whole, frac = ""] = trimmed.split(".");
  const cents = Number(whole) * 100 + Number(frac.padEnd(2, "0"));
  return cents >= MIN_SUBCENTS && cents <= MAX_SUBCENTS ? cents : null;
}

/** The two-place wire form of a valid ceiling, or null. */
export function normalizeCeiling(text: string): string | null {
  const cents = ceilingCents(text);
  if (cents === null) return null;
  return `${Math.floor(cents / 100)}.${String(cents % 100).padStart(2, "0")}`;
}

/** Subcents (1/10000 USD) as a locale-formatted two-place amount. */
export function formatSubcentsUsd(subcents: number): string {
  return formatNumber(subcents / 10_000, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}
