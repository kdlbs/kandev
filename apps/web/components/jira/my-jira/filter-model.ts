import type { JiraStatusCategory } from "@/lib/types/jira";

export type AssigneeFilter = "me" | "unassigned" | "anyone";
export type SortKey = "updated" | "created" | "priority";

export type FilterState = {
  projectKeys: string[];
  statusCategories: JiraStatusCategory[];
  assignee: AssigneeFilter;
  searchText: string;
  sort: SortKey;
};

export const DEFAULT_FILTERS: FilterState = {
  projectKeys: [],
  statusCategories: [],
  assignee: "me",
  searchText: "",
  sort: "updated",
};

export const STATUS_CATEGORY_OPTIONS: { value: JiraStatusCategory; label: string }[] = [
  { value: "new", label: "To Do" },
  { value: "indeterminate", label: "In Progress" },
  { value: "done", label: "Done" },
];

function quote(value: string): string {
  return `"${value.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
}

const TICKET_KEY_RE = /^[A-Z][A-Z0-9]+-\d+$/;

function searchClause(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  if (TICKET_KEY_RE.test(trimmed)) return `key = ${quote(trimmed)}`;
  return `text ~ ${quote(trimmed)}`;
}

function statusClause(categories: JiraStatusCategory[]): string | null {
  if (categories.length === 0) return null;
  const labels = categories.map((c) => {
    if (c === "new") return quote("To Do");
    if (c === "indeterminate") return quote("In Progress");
    return quote("Done");
  });
  return `statusCategory in (${labels.join(", ")})`;
}

function assigneeClause(a: AssigneeFilter): string | null {
  if (a === "me") return "assignee = currentUser()";
  if (a === "unassigned") return "assignee is EMPTY";
  return null;
}

function projectClause(keys: string[]): string | null {
  if (keys.length === 0) return null;
  return `project in (${keys.map(quote).join(", ")})`;
}

function sortClause(s: SortKey): string {
  if (s === "created") return "ORDER BY created DESC";
  if (s === "priority") return "ORDER BY priority DESC, updated DESC";
  return "ORDER BY updated DESC";
}

export function filtersToJql(f: FilterState): string {
  const clauses = [
    projectClause(f.projectKeys),
    statusClause(f.statusCategories),
    assigneeClause(f.assignee),
    searchClause(f.searchText),
  ].filter((c): c is string => c !== null);
  const where = clauses.join(" AND ");
  const order = sortClause(f.sort);
  return where ? `${where} ${order}` : order;
}

export function filtersEqual(a: FilterState, b: FilterState): boolean {
  return (
    a.assignee === b.assignee &&
    a.sort === b.sort &&
    a.searchText === b.searchText &&
    a.projectKeys.length === b.projectKeys.length &&
    a.projectKeys.every((k, i) => k === b.projectKeys[i]) &&
    a.statusCategories.length === b.statusCategories.length &&
    a.statusCategories.every((c, i) => c === b.statusCategories[i])
  );
}
