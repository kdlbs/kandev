import { describe, expect, it } from "vitest";
import { migrateView } from "./ui-slice";
import type { SidebarView } from "./sidebar-view-types";

function makeSidebarView(id: string, name: string): SidebarView {
  return {
    id,
    name,
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
    collapsedGroups: [],
  };
}

describe("migrateView archived compatibility", () => {
  it("drops legacy archived clauses while preserving the rest of the view", () => {
    const view = makeSidebarView("view-a", "Archived tasks");
    const archivedClause = {
      id: "archived",
      dimension: "archived",
      op: "is",
      value: true,
    } as unknown as SidebarView["filters"][number];
    view.filters = [
      archivedClause,
      { id: "title", dimension: "titleMatch", op: "matches", value: "keep" },
    ];
    view.sort = { key: "title", direction: "desc" };
    view.group = "repository";
    view.collapsedGroups = ["org/repo"];

    const migrated = migrateView(view);

    expect(migrated).toMatchObject({
      id: "view-a",
      name: "Archived tasks",
      sort: { key: "title", direction: "desc" },
      group: "repository",
      collapsedGroups: ["org/repo"],
    });
    expect(migrated.filters).toEqual([
      { id: "title", dimension: "titleMatch", op: "matches", value: "keep" },
    ]);
  });

  it("turns an archived-only legacy view into an unfiltered view", () => {
    const view = makeSidebarView("view-a", "Archived");
    view.filters = [
      {
        id: "archived",
        dimension: "archived",
        op: "is",
        value: true,
      } as unknown as SidebarView["filters"][number],
    ];

    expect(migrateView(view).filters).toEqual([]);
  });
});
