import { describe, it, expect } from "vitest";
import type { SidebarView } from "./sidebar-view-types";
import { fromApiSidebarView, toApiSidebarView } from "./sidebar-view-wire";

const view: SidebarView = {
  id: "v1",
  name: "My View",
  filters: [
    { id: "c1", dimension: "isPRReview", op: "is", value: true },
    { id: "c2", dimension: "state", op: "in", value: ["review", "in_progress"] },
    { id: "c3", dimension: "titleMatch", op: "matches", value: "fix " },
  ],
  sort: { key: "updatedAt", direction: "desc" },
  group: "workflow",
  collapsedGroups: ["backlog", "review"],
};

describe("sidebar view wire", () => {
  it("round-trips camelCase <-> snake_case", () => {
    const api = toApiSidebarView(view);
    expect(api.collapsed_groups).toEqual(view.collapsedGroups);
    expect(api.sort).toEqual(view.sort);
    expect(api.filters).toHaveLength(view.filters.length);
    const restored = fromApiSidebarView(api);
    expect(restored).toEqual(view);
  });

  it("defaults missing collapsed_groups to an empty array on read", () => {
    const restored = fromApiSidebarView({
      id: "v2",
      name: "Minimal",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "none",
      collapsed_groups: undefined as unknown as string[],
    });
    expect(restored.collapsedGroups).toEqual([]);
  });

  it("passes filter values through unchanged (bool / string / array)", () => {
    const api = toApiSidebarView(view);
    expect(api.filters[0].value).toBe(true);
    expect(api.filters[1].value).toEqual(["review", "in_progress"]);
    expect(api.filters[2].value).toBe("fix ");
  });
});
