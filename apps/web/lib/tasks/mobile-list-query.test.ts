import { describe, expect, it } from "vitest";
import type { SidebarTaskQuery } from "@/lib/types/http";
import type { FilterClause } from "@/lib/state/slices/ui/sidebar-view-types";
import { composeMobileListQuery, resolveMobileListLinkScope } from "./mobile-list-query";

const baseQuery: SidebarTaskQuery = {
  filters: [
    { dimension: "workflow", op: "is", value: "saved-workflow" },
    { dimension: "repository", op: "is", value: "saved-repository" },
    { dimension: "state", op: "is", value: "in_progress" },
  ],
  sort: {
    key: "running",
    direction: "desc",
    then_by: [
      { key: "color", color: "red", direction: "desc" },
      { key: "lastActivityAt", direction: "desc" },
    ],
  },
  group: "workflow",
  collapsed_group_keys: ["workflow:saved-workflow"],
  collapsed_task_ids: ["task-collapsed"],
  page: 3,
  page_size: 100,
  locale: "en",
};

describe("mobile saved-view result query", () => {
  it("resolves archive, workflow, repository, search, and legacy view links locally", () => {
    const scope = resolveMobileListLinkScope(
      new URLSearchParams(
        "archive=archived&workflow=all&repository=repo-1&q=literal%25_%26&sort=created_asc&group=workflow_step",
      ),
    );

    expect(scope).toEqual({
      query: "literal%_&",
      filters: [
        { dimension: "archived", op: "is", value: true },
        { dimension: "repository", op: "is", value: "repo-1" },
      ],
      replaceFilterDimensions: ["archived", "workflow", "repository"],
      sort: { key: "createdAt", direction: "asc" },
      group: "workflowStep",
    });
  });

  it("keeps all-archive and plugin-facet link scope separate from saved view rules", () => {
    expect(
      resolveMobileListLinkScope(
        new URLSearchParams("archive=all&sort=facet:tags:label&group=facet:tags:label"),
      ),
    ).toEqual({
      filters: [{ dimension: "archived", op: "in", value: [false, true] }],
      replaceFilterDimensions: ["archived"],
      facetSort: "facet:tags:label",
      facetGroup: "facet:tags:label",
    });
  });

  it("adds temporary clauses conjunctively and resets the requested page", () => {
    const result = composeMobileListQuery(baseQuery, {
      searchQuery: "needle%_",
      resultFilters: [{ dimension: "state", op: "is", value: "completed" }],
    });

    expect(result.filters).toEqual([
      ...baseQuery.filters,
      { dimension: "state", op: "is", value: "completed" },
    ]);
    expect(result.query).toBe("needle%_");
    expect(result.page).toBe(1);
    expect(result.sort).toEqual(baseQuery.sort);
    expect(baseQuery.page).toBe(3);
    expect(baseQuery.filters).toHaveLength(3);
  });

  it("omits editor-only clause IDs from the request", () => {
    const temporaryClause: FilterClause = {
      id: "temporary-state",
      dimension: "state",
      op: "is",
      value: "review",
    };
    const result = composeMobileListQuery(baseQuery, { resultFilters: [temporaryClause] });

    expect(result.filters.at(-1)).toEqual({
      dimension: "state",
      op: "is",
      value: "review",
    });
    expect("id" in (result.filters.at(-1) ?? {})).toBe(false);
  });

  it("replaces only link-owned filter dimensions without changing the saved view", () => {
    const result = composeMobileListQuery(baseQuery, {
      resultFilters: [{ dimension: "state", op: "is", value: "completed" }],
      linkScope: {
        query: "archive needle",
        filters: [{ dimension: "archived", op: "is", value: true }],
        replaceFilterDimensions: ["workflow", "repository", "archived"],
        sort: { key: "lastActivityAt", direction: "desc" },
        group: "none",
      },
    });

    expect(result.filters).toEqual([
      baseQuery.filters[2],
      { dimension: "archived", op: "is", value: true },
      { dimension: "state", op: "is", value: "completed" },
    ]);
    expect(result.query).toBe("archive needle");
    expect(result.sort).toEqual({ key: "lastActivityAt", direction: "desc" });
    expect(result.group).toBe("none");
    expect(baseQuery.filters[0]?.value).toBe("saved-workflow");
    expect(baseQuery.sort).toEqual({
      key: "running",
      direction: "desc",
      then_by: [
        { key: "color", color: "red", direction: "desc" },
        { key: "lastActivityAt", direction: "desc" },
      ],
    });
  });

  it("can explicitly remove a saved filter for an all-scope link", () => {
    const result = composeMobileListQuery(baseQuery, {
      linkScope: { replaceFilterDimensions: ["workflow", "repository", "archived"] },
    });

    expect(result.filters).toEqual([baseQuery.filters[2]]);
    expect(result.query).toBeUndefined();
  });
});

describe("mobile link repository filters", () => {
  it("resolves desktop List repository IDs to the shared sidebar repository value", () => {
    const scope = resolveMobileListLinkScope(
      new URLSearchParams("repository=repo-id"),
      new Map([["repo-id", "acme/widgets"]]),
    );

    expect(scope?.filters).toEqual([{ dimension: "repository", op: "is", value: "acme/widgets" }]);
  });
});
