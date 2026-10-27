import type { SidebarTaskQuery } from "@/lib/types/http";
import { parseTasksListGroup, parseTasksListSort } from "./tasks-list-options";

export type MobileListLinkScope = {
  query?: string;
  filters?: SidebarTaskQuery["filters"];
  replaceFilterDimensions?: readonly string[];
  sort?: SidebarTaskQuery["sort"];
  group?: string;
  facetSort?: string;
  facetGroup?: string;
};

export type MobileListQueryOptions = {
  searchQuery?: string;
  resultFilters?: SidebarTaskQuery["filters"];
  linkScope?: MobileListLinkScope;
};

export type MobileListFacetSettings = {
  options: ReadonlyArray<{ value: string; label: string }>;
  sort: string | null;
  group: string | null;
  onSortChange: (facetKey: string | null) => void;
  onGroupChange: (facetKey: string | null) => void;
  onReset: () => void;
};

function legacySortOverride(value: string): SidebarTaskQuery["sort"] | undefined {
  const sort = parseTasksListSort(value);
  if (sort.startsWith("facet:")) return undefined;
  if (sort.startsWith("created_"))
    return { key: "createdAt", direction: sort.endsWith("asc") ? "asc" : "desc" };
  if (sort.startsWith("title_"))
    return { key: "title", direction: sort.endsWith("asc") ? "asc" : "desc" };
  return { key: "updatedAt", direction: sort.endsWith("asc") ? "asc" : "desc" };
}

function legacyGroupOverride(value: string): string | undefined {
  const group = parseTasksListGroup(value);
  if (group.startsWith("facet:")) return undefined;
  return group === "workflow_step" ? "workflowStep" : group;
}

function appendArchiveOverride(
  params: URLSearchParams,
  filters: NonNullable<SidebarTaskQuery["filters"]>,
  replacedDimensions: string[],
): void {
  if (!params.has("archive")) return;
  replacedDimensions.push("archived");
  const archive = params.get("archive");
  filters.push(
    archive === "all"
      ? { dimension: "archived", op: "in", value: [false, true] }
      : { dimension: "archived", op: "is", value: archive === "archived" },
  );
}

function appendWorkflowRepositoryOverrides(
  params: URLSearchParams,
  filters: NonNullable<SidebarTaskQuery["filters"]>,
  replacedDimensions: string[],
  repositorySlugById?: ReadonlyMap<string, string>,
): void {
  for (const dimension of ["workflow", "repository"] as const) {
    if (!params.has(dimension)) continue;
    replacedDimensions.push(dimension);
    const value = params.get(dimension);
    if (value && value !== "all") {
      filters.push({
        dimension,
        op: "is",
        value: dimension === "repository" ? (repositorySlugById?.get(value) ?? value) : value,
      });
    }
  }
}

function hasMobileListLinkScope(params: URLSearchParams): boolean {
  return ["query", "q", "archive", "workflow", "repository", "sort", "group"].some((key) =>
    params.has(key),
  );
}

function createMobileListLinkScope(
  params: URLSearchParams,
  queryKey: string | undefined,
  filters: NonNullable<SidebarTaskQuery["filters"]>,
  replaceFilterDimensions: string[],
): MobileListLinkScope {
  const scope: MobileListLinkScope = {};
  if (queryKey) scope.query = params.get(queryKey) ?? "";
  if (filters.length) scope.filters = filters;
  if (replaceFilterDimensions.length) scope.replaceFilterDimensions = replaceFilterDimensions;

  const sortValue = params.get("sort");
  const groupValue = params.get("group");
  if (sortValue?.startsWith("facet:")) scope.facetSort = parseTasksListSort(sortValue);
  if (groupValue?.startsWith("facet:")) scope.facetGroup = parseTasksListGroup(groupValue);
  const sort = sortValue ? legacySortOverride(sortValue) : undefined;
  const group = groupValue ? legacyGroupOverride(groupValue) : undefined;
  if (sort) scope.sort = sort;
  if (group) scope.group = group;
  return scope;
}

/** Resolves standalone List URL state into a phone-only result override. */
export function resolveMobileListLinkScope(
  params: URLSearchParams,
  repositorySlugById?: ReadonlyMap<string, string>,
): MobileListLinkScope | null {
  const replaceFilterDimensions: string[] = [];
  const filters: SidebarTaskQuery["filters"] = [];
  appendArchiveOverride(params, filters, replaceFilterDimensions);
  appendWorkflowRepositoryOverrides(params, filters, replaceFilterDimensions, repositorySlugById);

  const queryKey = ["query", "q"].find((key) => params.has(key));
  if (!hasMobileListLinkScope(params)) return null;
  return createMobileListLinkScope(params, queryKey, filters, replaceFilterDimensions);
}

/** Composes request-only phone refinements over a shared saved view. */
export function composeMobileListQuery(
  savedQuery: SidebarTaskQuery,
  options: MobileListQueryOptions = {},
): SidebarTaskQuery {
  const linkScope = options.linkScope;
  const linkFilters = linkScope?.filters ?? [];
  const replacedDimensions = new Set([
    ...(linkScope?.replaceFilterDimensions ?? []),
    ...linkFilters.map((filter) => filter.dimension),
  ]);
  const filters = [
    ...savedQuery.filters.filter((filter) => !replacedDimensions.has(filter.dimension)),
    ...linkFilters,
    ...(options.resultFilters ?? []).map(({ dimension, op, value }) => ({
      dimension,
      op,
      value,
    })),
  ];
  const query = options.searchQuery ?? linkScope?.query ?? savedQuery.query;
  const result: SidebarTaskQuery = {
    ...savedQuery,
    filters,
    sort: linkScope?.sort ?? savedQuery.sort,
    group: linkScope?.group ?? savedQuery.group,
    page: 1,
  };
  if (query) result.query = query;
  else delete result.query;
  return result;
}
