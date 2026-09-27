import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

const NANOS_PER_MS = BigInt(1_000_000);

/**
 * Formats an item or row's age (ms) as "<h>h <m>m", or "<m>m" under an hour;
 * a negative or missing age (a reference time in the future, or absent) reads
 * as "0m" (Adoption decision 9, docs/plans/workspace-coordinator/
 * task-04-needs-you-queue-stalls.md).
 */
export function formatAge(ageMs: number | undefined): string {
  if (ageMs === undefined || ageMs <= 0) return "0m";
  const totalMinutes = Math.floor(ageMs / 60_000);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`;
}

/** Formats an epoch millisecond timestamp as a local "HH:mm" time. */
export function formatLocalTime(epochMs: number): string {
  const date = new Date(epochMs);
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  return `${hours}:${minutes}`;
}

/**
 * Truncates an error preview to 140 characters
 * (docs/specs/coordinator/system-design/needs-you.md#classification).
 */
export function truncatePreview(preview: string, maxLength = 140): string {
  return preview.length > maxLength ? preview.slice(0, maxLength) : preview;
}

/**
 * Formats a wire RFC3339 timestamp (e.g. a stall record's `last_event_at` or
 * `detected_at`) as a local "HH:mm" time. Returns undefined for a missing or
 * malformed value rather than trusting `Date.parse`'s normalization.
 */
export function formatEvidenceTime(value: string | undefined): string | undefined {
  const ns = parseStrictRfc3339Timestamp(value);
  if (ns === null) return undefined;
  return formatLocalTime(Number(ns / NANOS_PER_MS));
}
