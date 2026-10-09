import type {
  SidebarColorToken,
  SortCriterion,
  SortKey,
  SortRule,
  SortSpec,
} from "@/lib/state/slices/ui/sidebar-view-types";
import type { SidebarTaskQuery } from "@/lib/types/http";

export const MAX_SIDEBAR_SORT_RULES = 10;

const SORT_KEYS = new Set<SortKey>([
  "state",
  "updatedAt",
  "lastActivityAt",
  "createdAt",
  "title",
  "running",
  "color",
  "custom",
]);

const COLOR_TOKENS = new Set<SidebarColorToken>([
  "gray",
  "red",
  "orange",
  "yellow",
  "green",
  "cyan",
  "blue",
  "indigo",
  "purple",
  "pink",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function parseCriterion(value: unknown): SortRule | null {
  if (!isRecord(value)) return null;
  const key = value.key;
  const direction = value.direction;
  if (typeof key !== "string" || !SORT_KEYS.has(key as SortKey)) return null;
  if (direction !== "asc" && direction !== "desc") return null;
  if (key === "custom") return { key, direction };
  if (key === "color") {
    if (typeof value.color !== "string" || !COLOR_TOKENS.has(value.color as SidebarColorToken))
      return null;
    return { key, color: value.color as SidebarColorToken, direction };
  }
  if (value.color !== undefined) return null;
  return { key: key as Exclude<SortKey, "custom" | "color">, direction };
}

function sameRuleIdentity(a: SortRule, b: SortRule): boolean {
  if (a.key !== b.key) return false;
  return a.key !== "color" || (b.key === "color" && a.color === b.color);
}

/** Converts persisted and partially-edited values to a safe ordered rule chain. */
export function normalizeSidebarSort(value: unknown): {
  sort: SortSpec;
  droppedRuleCount: number;
} {
  if (!isRecord(value)) {
    return { sort: { key: "state", direction: "asc" }, droppedRuleCount: 0 };
  }
  if (value.key === "runningFirstActivity") {
    return {
      sort: {
        key: "running",
        direction: "desc",
        thenBy: [{ key: "lastActivityAt", direction: "desc" }],
      },
      droppedRuleCount: 0,
    };
  }

  const candidates = [value, ...(Array.isArray(value.thenBy) ? value.thenBy : [])];
  const rules: SortRule[] = [];
  let droppedRuleCount = 0;
  for (const candidate of candidates) {
    const rule = parseCriterion(candidate);
    if (!rule || rules.length >= MAX_SIDEBAR_SORT_RULES) {
      droppedRuleCount += 1;
      continue;
    }
    if (rule.key === "custom" && candidates.length > 1) {
      droppedRuleCount += 1;
      continue;
    }
    if (rules.some((existing) => sameRuleIdentity(existing, rule))) {
      droppedRuleCount += 1;
      continue;
    }
    rules.push(rule);
  }
  if (rules.length === 0) {
    return { sort: { key: "state", direction: "asc" }, droppedRuleCount };
  }
  const [primary, ...remaining] = rules;
  const thenBy = remaining.filter((rule): rule is SortCriterion => rule.key !== "custom");
  return {
    sort: { ...primary, ...(thenBy.length ? { thenBy } : {}) },
    droppedRuleCount,
  };
}

export function sidebarSortRules(sort: SortSpec): SortRule[] {
  const normalized = normalizeSidebarSort(sort).sort;
  const { thenBy = [], ...primary } = normalized;
  return [primary, ...thenBy];
}

export function sidebarSortHasKey(sort: SortSpec, key: SortKey): boolean {
  return sidebarSortRules(sort).some((rule) => rule.key === key);
}

export function sidebarSortToWire(sort: SortSpec): SidebarTaskQuery["sort"] {
  return {
    key: sort.key,
    direction: sort.direction,
    ...(sort.color ? { color: sort.color } : {}),
    ...(sort.thenBy?.length
      ? {
          then_by: sort.thenBy.map(({ key, direction, color }) => ({
            key,
            direction,
            ...(color ? { color } : {}),
          })),
        }
      : {}),
  };
}

export function sidebarSortFromWire(sort: SidebarTaskQuery["sort"]): SortSpec {
  return normalizeSidebarSort(sidebarSortFromWireRaw(sort)).sort;
}

/** Keeps untrusted settings intact until saved-view migration can report dropped rules. */
export function sidebarSortFromWireRaw(sort: SidebarTaskQuery["sort"]): SortSpec {
  return {
    key: sort.key,
    direction: sort.direction,
    ...(sort.color ? { color: sort.color as SortSpec["color"] } : {}),
    ...(sort.then_by
      ? {
          thenBy: sort.then_by.map((rule) => ({
            key: rule.key as NonNullable<SortSpec["thenBy"]>[number]["key"],
            direction: rule.direction as NonNullable<SortSpec["thenBy"]>[number]["direction"],
            ...(rule.color
              ? { color: rule.color as NonNullable<SortSpec["thenBy"]>[number]["color"] }
              : {}),
          })),
        }
      : {}),
  } as SortSpec;
}
