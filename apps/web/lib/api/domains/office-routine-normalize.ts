import type { RoutineTrigger, RoutineTriggerKind } from "@/lib/state/slices/office/types";

function rawField(record: Record<string, unknown>, camelKey: string, snakeKey: string): unknown {
  return record[camelKey] ?? record[snakeKey];
}

function stringField(record: Record<string, unknown>, camelKey: string, snakeKey: string): string {
  const value = rawField(record, camelKey, snakeKey);
  return typeof value === "string" ? value : "";
}

function booleanField(
  record: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
): boolean {
  const value = rawField(record, camelKey, snakeKey);
  return typeof value === "boolean" ? value : false;
}

/** Carries a non-empty string through unchanged; every other wire value, including `""`, becomes `undefined`. */
function timestampField(
  record: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
): string | undefined {
  const value = rawField(record, camelKey, snakeKey);
  return typeof value === "string" && value !== "" ? value : undefined;
}

/**
 * Converts one wire trigger to the domain shape. Returns `null` when the
 * normalized `id` is empty: `id` is the delete target and the primary-trigger
 * selector's tiebreak column, so a trigger a caller can't address or order is
 * dropped rather than returned. Built from an explicit field list rather than
 * a spread, so `secret` never reaches component state.
 */
export function normalizeRoutineTrigger(raw: unknown): RoutineTrigger | null {
  const record = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};
  const id = stringField(record, "id", "id");
  if (id === "") return null;
  return {
    id,
    routineId: stringField(record, "routineId", "routine_id"),
    kind: stringField(record, "kind", "kind"),
    cronExpression: stringField(record, "cronExpression", "cron_expression"),
    timezone: stringField(record, "timezone", "timezone"),
    publicId: stringField(record, "publicId", "public_id"),
    signingMode: stringField(record, "signingMode", "signing_mode"),
    nextRunAt: timestampField(record, "nextRunAt", "next_run_at"),
    lastFiredAt: timestampField(record, "lastFiredAt", "last_fired_at"),
    enabled: booleanField(record, "enabled", "enabled"),
    createdAt: stringField(record, "createdAt", "created_at"),
    updatedAt: stringField(record, "updatedAt", "updated_at"),
  };
}

/**
 * Normalizes a trigger list response: non-object elements are dropped, order
 * is preserved, and a non-array input degrades to an empty array rather than
 * throwing.
 */
export function normalizeRoutineTriggerList(raw: unknown): RoutineTrigger[] {
  if (!Array.isArray(raw)) return [];
  const result: RoutineTrigger[] = [];
  for (const item of raw) {
    if (!item || typeof item !== "object") continue;
    const normalized = normalizeRoutineTrigger(item);
    if (normalized) result.push(normalized);
  }
  return result;
}

/**
 * The trigger create request body, declared for this purpose rather than
 * derived from `RoutineTrigger`: `secret` is write-only and `routineId` is
 * carried by the path argument, not the body. Server-owned fields (`id`,
 * `enabled`, `nextRunAt`, `lastFiredAt`, `createdAt`, `updatedAt`) are not
 * accepted.
 */
export type CreateRoutineTriggerInput = {
  kind: RoutineTriggerKind;
  cronExpression?: string;
  timezone?: string;
  publicId?: string;
  signingMode?: string;
  secret?: string;
};

const OPTIONAL_WIRE_FIELDS: ReadonlyArray<[keyof CreateRoutineTriggerInput, string]> = [
  ["cronExpression", "cron_expression"],
  ["timezone", "timezone"],
  ["publicId", "public_id"],
  ["signingMode", "signing_mode"],
  ["secret", "secret"],
];

/**
 * Serializes a create input to the snake_case wire body. A field is emitted
 * only when the caller supplied it (present with a value other than
 * `undefined`); an explicit empty string is emitted as `""`, never omitted.
 * No camelCase key is ever emitted alongside its snake_case counterpart.
 */
export function serializeRoutineTriggerInput(
  input: CreateRoutineTriggerInput,
): Record<string, unknown> {
  const body: Record<string, unknown> = { kind: input.kind };
  for (const [camelKey, wireKey] of OPTIONAL_WIRE_FIELDS) {
    const value = input[camelKey];
    if (value !== undefined) body[wireKey] = value;
  }
  return body;
}
