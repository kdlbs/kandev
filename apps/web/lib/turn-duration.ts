import type { Message, Turn } from "@/lib/types/http";

export type PromptDurationUnits = {
  s: string;
  m: string;
  h: string;
};

/**
 * Parse an RFC3339/RFC3339Nano timestamp into a full BigInt epoch-nanosecond
 * key, returning `null` for missing or unparseable values. The variable-width
 * fraction is removed before whole-second parsing (Date.parse only has
 * millisecond precision and rounds the sub-ms remainder), then re-attached as
 * the digit run right-padded to nine digits — matching the backend's
 * `YYYY-MM-DD HH:MM:SS.ffffff` UTC microsecond key after floor division by
 * 1000n. Offsets (e.g. `+02:00`) are normalized by Date.parse to UTC; they
 * are whole seconds, so the fraction is preserved unchanged.
 */
function epochNanoseconds(value: string | undefined): bigint | null {
  if (!value) return null;
  const dot = value.indexOf(".");
  let wholePart = value;
  let fractionDigits = "";
  if (dot !== -1) {
    const rest = value.slice(dot + 1);
    const digitMatch = /^\d+/.exec(rest);
    fractionDigits = digitMatch ? digitMatch[0] : "";
    // Remove only the fraction digits; the zone suffix (Z or ±HH:MM) stays in
    // the whole part so Date.parse still normalizes the offset to UTC.
    wholePart = value.slice(0, dot) + rest.slice(fractionDigits.length);
  }
  const parsed = Date.parse(wholePart);
  if (Number.isNaN(parsed)) return null;
  // The whole part has no fraction, so the ms value is a whole second.
  const seconds = BigInt(Math.floor(parsed / 1000));
  const fractionNs = BigInt(fractionDigits.padEnd(9, "0").slice(0, 9) || "0");
  return seconds * BigInt(1_000_000_000) + fractionNs;
}

/**
 * Return a completed user prompt turn's wall-clock duration in whole seconds.
 * The bound is the turn's own completion timestamp — a caller that needs a
 * next-prompt bound derives it separately. Timestamps are parsed at full
 * nanosecond precision, floored to seconds, and clamped at zero so clock skew
 * cannot produce a negative duration.
 */
export function messageTurnDurationSeconds(message: Message, turn: Turn | null): number | null {
  if (
    message.author_type !== "user" ||
    !message.turn_id ||
    turn === null ||
    turn.id !== message.turn_id ||
    turn.session_id !== message.session_id
  ) {
    return null;
  }

  const createdAt = epochNanoseconds(message.created_at);
  const completedAt = epochNanoseconds(turn.completed_at);
  if (createdAt === null || completedAt === null) return null;

  const durationNs = completedAt - createdAt;
  return Number(durationNs < BigInt(0) ? BigInt(0) : durationNs / BigInt(1_000_000_000));
}

/** Format a duration in seconds as a compact `h m s` string using the given unit labels, omitting empty hour/minute parts. */
export function formatPromptDuration(seconds: number, units: PromptDurationUnits): string {
  const totalSeconds = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remainingSeconds = totalSeconds % 60;

  if (hours > 0) return `${hours}${units.h} ${minutes}${units.m} ${remainingSeconds}${units.s}`;
  if (minutes > 0) return `${minutes}${units.m} ${remainingSeconds}${units.s}`;
  return `${remainingSeconds}${units.s}`;
}
