import { describe, expect, it } from "vitest";
import { migrateSidebarViewDraft, migrateView } from "./ui-slice";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";

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

function makeSidebarDraft(baseViewId = "view-a"): SidebarViewDraft {
  return {
    baseViewId,
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
    taskRow: {
      detailsEnabled: true,
      detailOrder: ["relative_time", "repository", "pull_request_number"],
      visibleDetails: ["relative_time", "repository", "pull_request_number"],
      trailing: "git_changes",
    },
  };
}

describe("migrateView archived compatibility", () => {
  it("preserves archived clauses while preserving the rest of the view", () => {
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
    expect(migrated.filters).toEqual(view.filters);
  });

  it("preserves an archived-only view", () => {
    const view = makeSidebarView("view-a", "Archived");
    view.filters = [
      {
        id: "archived",
        dimension: "archived",
        op: "is",
        value: true,
      } as unknown as SidebarView["filters"][number],
    ];

    expect(migrateView(view).filters).toEqual(view.filters);
  });
});

describe("migrateSidebarViewDraft archived compatibility", () => {
  it("preserves archived clauses while preserving draft state", () => {
    const draft = makeSidebarDraft();
    draft.filters = [
      {
        id: "archived",
        dimension: "archived",
        op: "is",
        value: true,
      } as unknown as SidebarViewDraft["filters"][number],
      { id: "title", dimension: "titleMatch", op: "matches", value: "keep" },
    ];
    draft.sort = { key: "title", direction: "desc" };
    draft.group = "repository";

    expect(migrateSidebarViewDraft(draft)).toEqual(draft);
  });
});

describe("migrate sidebar activity sort", () => {
  it("preserves lastActivityAt on saved views and drafts", () => {
    const view = makeSidebarView("view-activity", "Recent activity");
    view.sort = { key: "lastActivityAt", direction: "desc" };
    const draft = makeSidebarDraft("view-activity");
    draft.sort = { key: "lastActivityAt", direction: "asc" };

    expect(migrateView(view).sort).toEqual(view.sort);
    expect(migrateSidebarViewDraft(draft).sort).toEqual(draft.sort);
  });
});
