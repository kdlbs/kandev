import { describe, it, expect } from "vitest";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import {
  fromApiSidebarDraft,
  fromApiSidebarView,
  toApiSidebarDraft,
  toApiSidebarView,
} from "./sidebar-view-wire";

const view: SidebarView = {
  id: "v1",
  name: "My View",
  filters: [
    { id: "c1", dimension: "isPRReview", op: "is", value: true },
    { id: "c2", dimension: "state", op: "in", value: ["review", "in_progress"] },
    { id: "c3", dimension: "titleMatch", op: "matches", value: "fix " },
  ],
  sort: { key: "lastActivityAt", direction: "desc" },
  group: "workflow",
  collapsedGroups: ["backlog", "review"],
  taskRow: {
    detailsEnabled: true,
    detailOrder: ["relative_time", "repository", "pull_request_number"],
    visibleDetails: ["relative_time", "repository", "pull_request_number"],
    trailing: "git_changes",
  },
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

  it("round-trips the saved task-row presentation", () => {
    const taskRow: NonNullable<SidebarView["taskRow"]> = {
      detailsEnabled: true,
      detailOrder: ["relative_time", "repository", "pull_request_number"],
      visibleDetails: ["repository", "pull_request_number"],
      trailing: "change_request_status" as const,
    };
    const viewWithTaskRow: SidebarView = { ...view, taskRow };
    const api = toApiSidebarView(viewWithTaskRow);

    expect(api.task_row).toEqual({
      details_enabled: true,
      detail_order: ["relative_time", "repository", "pull_request_number"],
      visible_details: ["repository", "pull_request_number"],
      trailing: "change_request_status",
    });
    const restored = fromApiSidebarView(api);
    expect(restored.taskRow).toEqual({
      detailsEnabled: true,
      detailOrder: ["relative_time", "repository", "pull_request_number"],
      visibleDetails: ["repository", "pull_request_number"],
      trailing: "change_request_status",
    });
  });

  it("round-trips the task-row presentation on a saved-view draft", () => {
    const draft: SidebarViewDraft = {
      baseViewId: view.id,
      filters: view.filters,
      sort: view.sort,
      group: view.group,
      taskRow: {
        detailsEnabled: false,
        detailOrder: ["pull_request_number", "relative_time", "repository"],
        visibleDetails: ["pull_request_number"],
        trailing: "none",
      },
    };

    const api = toApiSidebarDraft(draft);
    expect(api.task_row?.details_enabled).toBe(false);
    expect(fromApiSidebarDraft(api)).toEqual(draft);
  });

  it("normalizes a missing task-row presentation to the current layout", () => {
    const restored = fromApiSidebarView({
      id: "v3",
      name: "Legacy",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "none",
      collapsed_groups: [],
    });

    expect(restored.taskRow).toEqual({
      detailsEnabled: true,
      detailOrder: ["relative_time", "repository", "pull_request_number"],
      visibleDetails: ["relative_time", "repository", "pull_request_number"],
      trailing: "git_changes",
    });
  });
});
