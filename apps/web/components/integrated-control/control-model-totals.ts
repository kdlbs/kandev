import type { NativeModel } from "@/lib/types/control-native";

export const TOKEN_FIELDS = [
  "tokens_in",
  "tokens_out",
  "tokens_cached_read",
  "tokens_cached_write",
  "tokens_total",
] as const;
export type TokenField = (typeof TOKEN_FIELDS)[number];

/** A summed measurement: null when no contributing record reported it. */
export type Measured = { value: number | null; partial: boolean };

export type ModelWorkspaceShare = {
  workspaceId: string;
  events: number;
  cost: Measured;
};

export type ModelTotal = {
  /** Empty when the record names no concrete model (absent or a dynamic policy). */
  model: string;
  providers: string[];
  events: number;
  tokens: Record<TokenField, Measured>;
  cost: Measured;
  workspaces: ModelWorkspaceShare[];
};

const UNREPORTED = new Set(["", "unknown", "unavailable", "unpriced"]);
const reported = (source: string) => !UNREPORTED.has(source.trim().toLowerCase());

export function measurementKnown(model: NativeModel): boolean {
  return reported(model.measurement_source) && reported(model.usage_completeness);
}
export function costKnown(model: NativeModel): boolean {
  return reported(model.cost_source);
}

function add(sum: Measured | undefined, value: number | null): Measured {
  if (!sum) return { value, partial: false };
  if (value === null) return { value: sum.value, partial: sum.partial || sum.value !== null };
  if (sum.value === null) return { value, partial: true };
  return { value: sum.value + value, partial: sum.partial };
}

export function modelName(model: string): string {
  return model && model.toLowerCase() !== "dynamic" ? model : "";
}

/**
 * One row per model across workspaces. A token or cost figure stays unknown
 * unless a record reported it; when only some records did, the sum is marked
 * partial instead of being presented as complete. Rows sort by cost, then by
 * event count.
 */
export function modelTotals(
  rows: Array<{ workspaceId: string; models: NativeModel[] }>,
): ModelTotal[] {
  const totals = new Map<string, ModelTotal>();
  for (const { workspaceId, models } of rows) {
    for (const record of models) {
      const name = modelName(record.model);
      const known = measurementKnown(record);
      const cost = costKnown(record) ? record.cost_subcents : null;
      const current = totals.get(name);
      const tokens = Object.fromEntries(
        TOKEN_FIELDS.map((field) => [
          field,
          add(current?.tokens[field], known ? record[field] : null),
        ]),
      ) as Record<TokenField, Measured>;
      const providers = new Set(current?.providers ?? []);
      if (record.provider) providers.add(record.provider);
      const workspaces = [...(current?.workspaces ?? [])];
      const share = workspaces.find((item) => item.workspaceId === workspaceId);
      if (share) {
        share.events += record.event_count;
        share.cost = add(share.cost, cost);
      } else
        workspaces.push({ workspaceId, events: record.event_count, cost: add(undefined, cost) });
      totals.set(name, {
        model: name,
        providers: [...providers].sort(),
        events: (current?.events ?? 0) + record.event_count,
        tokens,
        cost: add(current?.cost, cost),
        workspaces,
      });
    }
  }
  return [...totals.values()].sort(
    (a, b) => (b.cost.value ?? -1) - (a.cost.value ?? -1) || b.events - a.events,
  );
}

/** Token columns worth a column: at least one row has a reported value. */
export function visibleTokenFields(rows: ModelTotal[]): TokenField[] {
  return TOKEN_FIELDS.filter((field) => rows.some((row) => row.tokens[field].value !== null));
}
