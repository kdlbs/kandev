import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type { SortRule, SortSpec } from "@/lib/state/slices/ui/sidebar-view-types";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import {
  getStateBucket,
  STATE_BUCKET_ORDER,
  type EffectiveTaskTreeState,
} from "./effective-task-tree-state";
import { sqliteLower } from "./sidebar-local-filter";
import { sidebarSortRules } from "./sidebar-sort-chain";
import { sidebarColorMatches } from "./sidebar-color-rank";

/** UTF-8 BINARY order, including astral code points that differ from UTF-16 order. */
export function sqliteBinary(left: string, right: string): number {
  if (left === right) return 0;
  const a = Array.from(left, (char) => char.codePointAt(0)!);
  const b = Array.from(right, (char) => char.codePointAt(0)!);
  for (let index = 0; index < Math.min(a.length, b.length); index++) {
    if (a[index] !== b[index]) return a[index] - b[index];
  }
  return a.length - b.length;
}

export function sqliteNoCase(left: string, right: string): number {
  return sqliteBinary(sqliteLower(left), sqliteLower(right));
}

export function sqliteActivityKey(value: string): string {
  const wire = value.replace(" ", "T").replace(/([+-]\d{2})$/, "$1:00");
  const parsed = parseStrictRfc3339Timestamp(wire);
  if (parsed === null) return value;
  const billion = BigInt(1_000_000_000);
  const fraction = ((parsed % billion) + billion) % billion;
  const seconds = (parsed - fraction) / billion;
  return (
    new Date(Number(seconds) * 1000).toISOString().slice(0, 19) +
    "." +
    String(fraction).padStart(9, "0")
  );
}

/** Task timestamps are stored as Go's SQLite datetime text, independently of summary timestamps. */
export function sqliteTaskTime(value: string | undefined): string {
  return (value ?? "").replace("T", " ").replace(/Z$/, "+00:00");
}

export function idOrder(ids: string[]): (id: string) => number {
  const positions = new Map<string, number>();
  ids.forEach((id, index) => {
    if (!positions.has(id)) positions.set(id, index);
  });
  return (id) => positions.get(id) ?? ids.length;
}

export type LocalTaskComparatorOptions = {
  sort: SortSpec;
  orderedIds: string[];
  states: ReadonlyMap<string, EffectiveTaskTreeState>;
  activities: ReadonlyMap<string, string>;
  running: ReadonlyMap<string, boolean>;
  colors: ReadonlyMap<string, string | null>;
};

type LocalSortValues = Omit<LocalTaskComparatorOptions, "sort" | "orderedIds"> & {
  order: (id: string) => number;
};

function compareLocalState(
  a: TaskSwitcherItem,
  b: TaskSwitcherItem,
  values: LocalSortValues,
): number {
  const left = values.states.get(a.id)?.bucket ?? getStateBucket(a);
  const right = values.states.get(b.id)?.bucket ?? getStateBucket(b);
  return STATE_BUCKET_ORDER[left] - STATE_BUCKET_ORDER[right];
}

function compareLocalColor(
  color: string | undefined,
  a: TaskSwitcherItem,
  b: TaskSwitcherItem,
  values: LocalSortValues,
): number {
  const left = sidebarColorMatches(values.colors.get(a.id), color ?? "");
  const right = sidebarColorMatches(values.colors.get(b.id), color ?? "");
  return Number(left) - Number(right);
}

function compareLocalCustom(
  a: TaskSwitcherItem,
  b: TaskSwitcherItem,
  order: LocalSortValues["order"],
): number {
  return (
    order(a.id) - order(b.id) ||
    sqliteBinary(sqliteTaskTime(b.createdAt), sqliteTaskTime(a.createdAt))
  );
}

function compareLocalRule(
  rule: SortRule,
  a: TaskSwitcherItem,
  b: TaskSwitcherItem,
  values: LocalSortValues,
) {
  switch (rule.key) {
    case "state":
      return compareLocalState(a, b, values);
    case "title":
      return sqliteNoCase(a.title, b.title);
    case "createdAt":
      return sqliteBinary(sqliteTaskTime(a.createdAt), sqliteTaskTime(b.createdAt));
    case "updatedAt":
      return sqliteBinary(sqliteTaskTime(a.updatedAt), sqliteTaskTime(b.updatedAt));
    case "lastActivityAt":
      return sqliteBinary(values.activities.get(a.id) ?? "", values.activities.get(b.id) ?? "");
    case "running":
      return Number(values.running.get(a.id) === true) - Number(values.running.get(b.id) === true);
    case "color":
      return compareLocalColor(rule.color, a, b, values);
    case "custom":
      return compareLocalCustom(a, b, values.order);
  }
}

function compareLocalCanonical(a: TaskSwitcherItem, b: TaskSwitcherItem): number {
  return (
    sqliteBinary(sqliteTaskTime(b.updatedAt), sqliteTaskTime(a.updatedAt)) ||
    sqliteNoCase(a.title, b.title) ||
    sqliteBinary(a.id, b.id)
  );
}

export function localTaskComparator(options: LocalTaskComparatorOptions) {
  const { sort, orderedIds, ...values } = options;
  const order = idOrder(orderedIds);
  const localValues = { ...values, order };
  const rules = sidebarSortRules(sort);
  return (a: TaskSwitcherItem, b: TaskSwitcherItem) => {
    for (const rule of rules) {
      const direction = rule.key !== "custom" && rule.direction === "desc" ? -1 : 1;
      const result = compareLocalRule(rule, a, b, localValues) * direction;
      if (result !== 0) return result;
    }
    return compareLocalCanonical(a, b);
  };
}
