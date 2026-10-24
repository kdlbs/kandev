import { describe, expect, it } from "vitest";
import {
  canUseTasksListBootData,
  resolveTasksListQuery,
  tasksListArchiveParams,
  tasksListFilterStatus,
} from "./tasks-list-query";

const defaults = { workflowId: "saved-flow", repositoryId: "saved-repo" };

describe("workspace task list query", () => {
  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-001.2 AC-TASKS-LIST-ARCHIVE-FILTERS-002.5
  it("uses explicit All scope instead of portable defaults for an archive link", () => {
    expect(
      resolveTasksListQuery(
        new URLSearchParams("archive=archived&workflow=all&repository=all"),
        defaults,
      ),
    ).toEqual({ archiveMode: "archived", searchQuery: "", workflowId: null, repositoryId: null });
  });

  it("inherits ordinary saved scope without normalizing literal search text", () => {
    expect(resolveTasksListQuery(new URLSearchParams("q=100%25_"), defaults)).toEqual({
      archiveMode: "active",
      searchQuery: "100%_",
      ...defaults,
    });
  });

  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-002.1
  it.each([
    ["active", {}],
    ["archived", { onlyArchived: true }],
    ["all", { includeArchived: true }],
  ] as const)("maps %s to server archive membership", (mode, expected) => {
    expect(tasksListArchiveParams(mode)).toEqual(expected);
  });

  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
  it("defaults an unsupported archive mode to Active without dropping explicit IDs", () => {
    expect(
      resolveTasksListQuery(new URLSearchParams("archive=invalid&workflow=unavailable"), defaults),
    ).toMatchObject({ archiveMode: "active", workflowId: "unavailable" });
  });

  // @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.2
  it("admits boot rows only for the same workspace and ordinary saved query", () => {
    expect(canUseTasksListBootData(new URLSearchParams(), "ws", "ws")).toBe(true);
    for (const query of ["archive=archived", "q=needle", "workflow=all", "repository=all"]) {
      expect(canUseTasksListBootData(new URLSearchParams(query), "ws", "ws")).toBe(false);
    }
    expect(canUseTasksListBootData(new URLSearchParams(), "old", "new")).toBe(false);
  });

  it("distinguishes missing scope, pending metadata, and failed metadata", () => {
    const scope = {
      query: { workflowId: "missing", repositoryId: null },
      workflowIds: [],
      repositoryIds: [],
      workflowReady: false,
      repositoryReady: true,
      workflowFailed: false,
      repositoryFailed: false,
    };
    expect(tasksListFilterStatus(scope)).toMatchObject({
      metadataLoading: true,
      unavailableFilter: null,
    });
    expect(tasksListFilterStatus({ ...scope, workflowReady: true })).toMatchObject({
      unavailableFilter: "workflow",
    });
    expect(tasksListFilterStatus({ ...scope, workflowFailed: true })).toMatchObject({
      metadataLoading: false,
      metadataError: true,
      unavailableFilter: null,
    });
  });

  it("does not broaden an explicitly empty workflow identifier", () => {
    expect(
      tasksListFilterStatus({
        query: { workflowId: "", repositoryId: null },
        workflowIds: [],
        repositoryIds: [],
        workflowReady: true,
        repositoryReady: true,
        workflowFailed: false,
        repositoryFailed: false,
      }),
    ).toMatchObject({ unavailableFilter: "workflow" });
  });
});
