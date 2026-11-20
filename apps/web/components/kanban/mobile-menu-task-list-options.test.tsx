import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TASKS_LIST_GROUP_OPTIONS, TASKS_LIST_SORT_OPTIONS } from "@/lib/tasks/tasks-list-options";
import { transform } from "@/scripts/lib/pseudo-locale.mjs";
import en from "@/src/locales/en/kanban.json";
import pt from "@/src/locales/pt-pt/kanban.json";
import zhcn from "@/src/locales/zh-cn/kanban.json";
import zhhk from "@/src/locales/zh-hk/kanban.json";
import zhtw from "@/src/locales/zh-tw/kanban.json";
import ja from "@/src/locales/ja/kanban.json";
import ko from "@/src/locales/ko/kanban.json";
import pseudo from "@/src/locales/pseudo/kanban.json";
import { MobileTasksListOptions } from "./mobile-menu-task-list-options";

const facetOptions = [{ value: "facet:plugin:tags", label: "tasks:groupByState" }];
const CATALOG_KEY = "groupTasksIntoSectionsByWorkflowStep";

afterEach(cleanup);

describe("phone task-list options", () => {
  function renderOptions(facets = facetOptions) {
    render(
      <MobileTasksListOptions
        options={{
          sort: "updated_desc",
          onSortChange: vi.fn(),
          group: "workflow_step",
          onGroupChange: vi.fn(),
          facetOptions: facets,
        }}
      />,
    );
  }

  it("renders catalog-key-shaped labels verbatim in both selector menus", () => {
    renderOptions();
    for (const id of ["mobile-tasks-list-sort", "mobile-tasks-list-group"]) {
      fireEvent.click(screen.getByTestId(id));
      const listbox = screen.getByRole("listbox");
      expect(within(listbox).getByRole("option", { name: "tasks:groupByState" })).toBeTruthy();
      fireEvent.keyDown(listbox, { key: "Escape" });
    }
    expect(screen.getByText(en[CATALOG_KEY])).toBeTruthy();
  });

  it("renders only built-in options when no facet is registered", () => {
    renderOptions([]);
    for (const [id, count] of [
      ["mobile-tasks-list-sort", TASKS_LIST_SORT_OPTIONS.length],
      ["mobile-tasks-list-group", TASKS_LIST_GROUP_OPTIONS.length],
    ] as const) {
      fireEvent.click(screen.getByTestId(id));
      expect(within(screen.getByRole("listbox")).getAllByRole("option")).toHaveLength(count);
      fireEvent.keyDown(screen.getByRole("listbox"), { key: "Escape" });
    }
  });

  it("names plugin-provided values in every shipped locale including regenerated pseudo", () => {
    const expected = [
      [en, "plugin-provided values"],
      [pt, "valores de plugin"],
      [zhcn, "插件"],
      [zhhk, "外掛"],
      [zhtw, "外掛"],
      [ja, "プラグイン"],
      [ko, "플러그인"],
    ] as const;
    for (const [catalog, phrase] of expected) {
      expect(catalog[CATALOG_KEY]).toContain(phrase);
    }
    expect(en[CATALOG_KEY]).not.toBe(
      "Group tasks into sections by workflow step, workflow, repository, or none.",
    );
    expect(pseudo[CATALOG_KEY]).toBe(transform(en[CATALOG_KEY]));
  });
});
