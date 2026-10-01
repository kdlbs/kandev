import type {
  DreamItem,
  DreamRating,
  OutcomeMeasure,
} from "@/lib/api/domains/coordinator-learning-api";
import { formatNumber } from "@/lib/i18n/formats";
import { formatSubcentsUsd } from "@/lib/coordinator/autonomy";

export const MEASURE_WINDOWS = [7, 30, 90] as const;
export const DEFAULT_MEASURE_DAYS = 30;
export const DREAM_RATINGS: readonly DreamRating[] = ["useful", "not_useful", "harmful"];

const WAIT_CONDITIONS = [
  "autonomy_off",
  "paused",
  "containment",
  "spend_unmeasurable",
  "ceiling",
  "no_evidence",
  "spacing",
  "overdue",
  "last_failed",
] as const;

/** A waiting or failed condition the copy knows; anything else has no fix text. */
export function knownCondition(condition: string | undefined): string | null {
  return condition && (WAIT_CONDITIONS as readonly string[]).includes(condition) ? condition : null;
}

/** Items the report lists under "Retire or supersede". */
export function isRetireItem(item: Pick<DreamItem, "kind">): boolean {
  return (
    item.kind === "standing_order_retire" ||
    item.kind === "note_retire" ||
    item.kind === "note_update"
  );
}

/** A measure with no value has no number to show, whatever its counts. */
export function measureHasValue(measure: OutcomeMeasure): boolean {
  return measure.value !== null && measure.null_reason === undefined;
}

export function formatPercent(value: number): string {
  return `${formatNumber(Math.round(value * 100))}%`;
}

export function formatDollars(value: number): string {
  return formatSubcentsUsd(Math.round(value * 10_000));
}

/** Seconds as the two most significant of days, hours and minutes. */
export function formatWait(seconds: number): { days: number; hours: number; minutes: number } {
  const total = Math.max(0, Math.round(seconds / 60));
  return {
    days: Math.floor(total / 1440),
    hours: Math.floor((total % 1440) / 60),
    minutes: total % 60,
  };
}
