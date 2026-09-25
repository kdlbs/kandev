import { describe, expect, it } from "vitest";
import type { SavedView } from "./use-saved-views";
import { DEFAULT_VIEW } from "./use-saved-views";
import { initialFilters, resolveInitialJiraView } from "./jira-default-view";

const customView: SavedView = {
  id: "custom:triaged",
  name: "Triaged tickets",
  filters: {
    projectKeys: ["CLIP"],
    statuses: ["In Development"],
    assignee: "anyone",
    searchText: "login",
    sort: "priority",
  },
  customJql: 'project = CLIP AND text ~ "login" ORDER BY priority DESC',
};

describe("Jira default view resolution", () => {
  it("restores an available saved custom view without changing its filters or JQL", () => {
    expect(resolveInitialJiraView(customView.id, [DEFAULT_VIEW, customView], "PROJ")).toEqual({
      filters: customView.filters,
      activeViewId: customView.id,
      customJql: customView.customJql,
      showJqlEditor: true,
    });
  });

  it("restores an available built-in view", () => {
    expect(
      resolveInitialJiraView(
        "builtin:unassigned",
        [
          DEFAULT_VIEW,
          {
            ...DEFAULT_VIEW,
            id: "builtin:unassigned",
            filters: { ...DEFAULT_VIEW.filters, assignee: "unassigned" },
          },
        ],
        "PROJ",
      ),
    ).toMatchObject({
      activeViewId: "builtin:unassigned",
      filters: { assignee: "unassigned", projectKeys: [] },
      customJql: null,
      showJqlEditor: false,
    });
  });

  it.each(["", "custom:deleted"])(
    "falls back when the default ID %s is absent",
    (defaultViewId) => {
      expect(resolveInitialJiraView(defaultViewId, [DEFAULT_VIEW], "PROJ")).toEqual({
        filters: initialFilters("PROJ"),
        activeViewId: DEFAULT_VIEW.id,
        customJql: null,
        showJqlEditor: false,
      });
    },
  );

  it("keeps the existing Assigned to me landing filter when no project key is configured", () => {
    expect(initialFilters("")).toEqual(DEFAULT_VIEW.filters);
    expect(initialFilters(" CLIP ")).toEqual({
      ...DEFAULT_VIEW.filters,
      projectKeys: ["CLIP"],
    });
  });
});
