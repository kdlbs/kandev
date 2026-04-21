import type { FilterDimension, FilterOp } from "@/lib/state/slices/ui/sidebar-view-types";

export type DimensionValueKind = "boolean" | "enum" | "text";

export type DimensionMeta = {
  dimension: FilterDimension;
  label: string;
  valueKind: DimensionValueKind;
  ops: FilterOp[];
  enumOptions?: Array<{ value: string; label: string }>;
  placeholder?: string;
  defaultOp: FilterOp;
  defaultValue: string | string[] | boolean;
};

const STATE_OPTIONS = [
  { value: "review", label: "Review" },
  { value: "in_progress", label: "In progress" },
  { value: "backlog", label: "Backlog" },
];

export const DIMENSION_METAS: DimensionMeta[] = [
  {
    dimension: "isPRReview",
    label: "PR review",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "isIssueWatch",
    label: "Issue watch",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "archived",
    label: "Archived",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasDiff",
    label: "Has diff",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "state",
    label: "State",
    valueKind: "enum",
    ops: ["in", "not_in", "is", "is_not"],
    enumOptions: STATE_OPTIONS,
    defaultOp: "in",
    defaultValue: ["review", "in_progress"],
  },
  {
    dimension: "workflow",
    label: "Workflow",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "workflowStep",
    label: "Workflow step",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "executorType",
    label: "Executor type",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "repository",
    label: "Repository",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "titleMatch",
    label: "Title",
    valueKind: "text",
    ops: ["matches", "not_matches"],
    placeholder: "Substring...",
    defaultOp: "matches",
    defaultValue: "",
  },
];

export function getDimensionMeta(dim: FilterDimension): DimensionMeta {
  const meta = DIMENSION_METAS.find((m) => m.dimension === dim);
  if (!meta) throw new Error(`Unknown filter dimension: ${dim}`);
  return meta;
}

export const OP_LABELS: Record<FilterOp, string> = {
  is: "is",
  is_not: "is not",
  in: "in",
  not_in: "not in",
  matches: "contains",
  not_matches: "does not contain",
};

export function getOpLabel(op: FilterOp, valueKind: DimensionValueKind): string {
  if (valueKind === "boolean") {
    if (op === "is") return "Show";
    if (op === "is_not") return "Hide";
  }
  return OP_LABELS[op];
}
