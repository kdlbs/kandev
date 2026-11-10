import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TASKS_LIST_SORT_OPTIONS, TASKS_LIST_GROUP_OPTIONS } from "@/lib/tasks/tasks-list-options";
import { TasksListControls } from "./tasks-list-controls";

const facetOptions = [{ value: "facet:plugin:tags", label: "tasks:groupByState" }];

afterEach(cleanup);

describe("desktop task-list facet options", () => {
  function renderControls(facets = facetOptions) {
    render(
      <TasksListControls
        archiveMode="active"
        onArchiveModeChange={vi.fn()}
        tasksListSort="updated_desc"
        onTasksListSortChange={vi.fn()}
        tasksListGroup="workflow_step"
        onTasksListGroupChange={vi.fn()}
        facetOptions={facets}
      />,
    );
  }

  it("shows a catalog-key-shaped plugin label verbatim in both menus", () => {
    renderControls();
    for (const id of ["tasks-list-sort", "tasks-list-group"]) {
      fireEvent.click(screen.getByTestId(id));
      const listbox = screen.getByRole("listbox");
      expect(within(listbox).getByRole("option", { name: "tasks:groupByState" })).toBeTruthy();
      fireEvent.keyDown(listbox, { key: "Escape" });
    }
  });

  it("has only built-in options without a registered facet", () => {
    renderControls([]);
    for (const [id, count] of [
      ["tasks-list-sort", TASKS_LIST_SORT_OPTIONS.length],
      ["tasks-list-group", TASKS_LIST_GROUP_OPTIONS.length],
    ] as const) {
      fireEvent.click(screen.getByTestId(id));
      expect(within(screen.getByRole("listbox")).getAllByRole("option")).toHaveLength(count);
      fireEvent.keyDown(screen.getByRole("listbox"), { key: "Escape" });
    }
  });
});
