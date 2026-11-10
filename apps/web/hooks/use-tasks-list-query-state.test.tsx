import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useTasksListQueryState } from "./use-tasks-list-query-state";

const persistScope = vi.fn();
const input = {
  workspaceId: "ws",
  workflowId: "saved-flow",
  repositoryId: "saved-repo",
  persistScope,
};
beforeEach(() => {
  persistScope.mockClear();
  window.history.replaceState({}, "", "/tasks?workspace=ws");
});
afterEach(cleanup);

it("does not replace a Home navigation while the workspace changes", () => {
  window.history.replaceState({}, "", "/tasks?archive=archived");
  const { rerender } = renderHook((props) => useTasksListQueryState(props), {
    initialProps: input,
  });
  window.history.replaceState({}, "", "/?workspaceId=new&home=overview");
  rerender({ ...input, workspaceId: "new" });
  expect(window.location.pathname).toBe("/");
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-001.3 AC-TASKS-LIST-ARCHIVE-FILTERS-004.1
it("restores explicit scope and history without saving portable defaults", () => {
  window.history.replaceState({}, "", "/tasks?archive=archived&workflow=all&repository=all");
  const { result } = renderHook(() => useTasksListQueryState(input));
  expect(result.current).toMatchObject({
    archiveMode: "archived",
    workflowId: null,
    repositoryId: null,
  });
  act(() => {
    window.history.replaceState({}, "", "/tasks?archive=all&workflow=other&q=needle");
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
  expect(result.current).toMatchObject({
    archiveMode: "all",
    workflowId: "other",
    searchQuery: "needle",
  });
  expect(persistScope).not.toHaveBeenCalled();
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-002.3 AC-TASKS-LIST-ARCHIVE-FILTERS-002.4
it("resets pagination with each filter and retains display choices when clearing", () => {
  window.history.replaceState(
    {},
    "",
    "/tasks?archive=archived&sort=title_desc&group=workflow_step",
  );
  const { result } = renderHook(() => useTasksListQueryState(input));
  act(() => result.current.setPagination({ pageIndex: 3, pageSize: 25 }));
  act(() => result.current.setSearchQuery("needle"));
  expect(result.current.pagination.pageIndex).toBe(0);
  act(() => result.current.clearFilters());
  expect(result.current).toMatchObject({
    archiveMode: "archived",
    searchQuery: "",
    workflowId: null,
    repositoryId: null,
  });
  expect(new URLSearchParams(window.location.search).get("group")).toBe("workflow_step");
  expect(new URLSearchParams(window.location.search).get("sort")).toBe("title_desc");
  expect(persistScope).toHaveBeenCalledWith({ workflowId: null, repositoryId: null });
});

it("saves only explicit scope edits and clears only the requested field", () => {
  const { result } = renderHook(() => useTasksListQueryState(input));
  act(() => result.current.setWorkflowId(null));
  expect(persistScope).toHaveBeenLastCalledWith({ workflowId: null });
  expect(result.current.repositoryId).toBe("saved-repo");
  act(() => result.current.setArchiveMode("archived"));
  expect(persistScope).toHaveBeenCalledTimes(1);
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.3
it("keeps archive mode and clears old scope when the workspace changes", () => {
  window.history.replaceState({}, "", "/tasks?archive=archived&q=old&workflow=old");
  const { result, rerender } = renderHook((props) => useTasksListQueryState(props), {
    initialProps: input,
  });
  act(() => {
    window.history.replaceState({}, "", "/tasks?workspace=new");
  });
  rerender({ ...input, workspaceId: "new" });
  expect(result.current).toMatchObject({
    archiveMode: "archived",
    searchQuery: "",
    workflowId: null,
    repositoryId: null,
  });
  expect(persistScope).not.toHaveBeenCalled();
});
