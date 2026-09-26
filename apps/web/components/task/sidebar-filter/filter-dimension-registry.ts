import { t } from "@/lib/i18n";
import type { FilterDimension, FilterOp } from "@/lib/state/slices/ui/sidebar-view-types";

export type DimensionValueKind = "boolean" | "enum" | "text";

// `labelKey` / `placeholderKey` hold catalog keys rather than copy: this table
// is module scope, so a resolved `t()` here would freeze at the boot locale.
// The `value` fields are persisted filter values and stay in English.
export type DimensionMeta = {
  dimension: FilterDimension;
  labelKey: string;
  valueKind: DimensionValueKind;
  ops: FilterOp[];
  enumOptions?: Array<{ value: string; labelKey: string }>;
  placeholderKey?: string;
  defaultOp: FilterOp;
  defaultValue: string | string[] | boolean;
};

const STATE_OPTIONS = [
  { value: "review", labelKey: "task:filterStateReview" },
  { value: "in_progress", labelKey: "task:filterStateInProgress" },
  { value: "backlog", labelKey: "task:filterStateBacklog" },
];

export const DIMENSION_METAS: DimensionMeta[] = [
  {
    dimension: "archived",
    labelKey: "task:filterDimensionArchived",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "isPRReview",
    labelKey: "task:filterDimensionPrReview",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "isIssueWatch",
    labelKey: "task:filterDimensionIssueWatch",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasDiff",
    labelKey: "task:filterDimensionHasDiff",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasPR",
    labelKey: "task:filterDimensionHasPr",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "state",
    labelKey: "task:filterDimensionState",
    valueKind: "enum",
    ops: ["in", "not_in", "is", "is_not"],
    enumOptions: STATE_OPTIONS,
    defaultOp: "in",
    defaultValue: ["review", "in_progress"],
  },
  {
    dimension: "workflow",
    labelKey: "task:filterDimensionWorkflow",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "workflowStep",
    labelKey: "task:filterDimensionWorkflowStep",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "executorType",
    labelKey: "task:filterDimensionExecutorType",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "repository",
    labelKey: "task:filterDimensionRepository",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "titleMatch",
    labelKey: "task:filterDimensionTitle",
    valueKind: "text",
    ops: ["matches", "not_matches"],
    placeholderKey: "task:filterTitlePlaceholder",
    defaultOp: "matches",
    defaultValue: "",
  },
];

export function getDimensionMeta(dim: FilterDimension): DimensionMeta {
  const meta = DIMENSION_METAS.find((m) => m.dimension === dim);
  if (!meta) throw new Error(`Unknown filter dimension: ${dim}`);
  return meta;
}

const OP_LABEL_KEYS: Record<FilterOp, string> = {
  is: "task:filterOpIs",
  is_not: "task:filterOpIsNot",
  in: "task:filterOpIn",
  not_in: "task:filterOpNotIn",
  matches: "task:filterOpContains",
  not_matches: "task:filterOpDoesNotContain",
};

// Module-level `t` is fine here: these run from render, not at import.
export function getOpLabel(op: FilterOp, valueKind: DimensionValueKind): string {
  if (valueKind === "boolean") {
    if (op === "is") return t("task:filterOpShow");
    if (op === "is_not") return t("task:filterOpHide");
  }
  return t(OP_LABEL_KEYS[op]);
}

/** Resolves a dimension's fixed enum options to displayable labels. */
export function getDimensionEnumOptions(
  meta: DimensionMeta,
): Array<{ value: string; label: string }> | undefined {
  return meta.enumOptions?.map((o) => ({ value: o.value, label: t(o.labelKey) }));
}
