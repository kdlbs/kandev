import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import type { FilterClause, SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import fixtureData from "../../../backend/internal/task/repository/testdata/sidebar-task-views.json";
import { applyView } from "./apply-view";

type SidebarViewConformanceFixture = {
  name: string;
  task_id_prefix: string;
  source_count: number;
  matching_task_index: number;
  title_prefix: string;
  matching_title: string;
  query: {
    filters: Array<Omit<FilterClause, "id">>;
    sort: SidebarView["sort"];
    group: SidebarView["group"];
  };
  expected_task_ids: string[];
};

const fixtures = fixtureData.fixtures as SidebarViewConformanceFixture[];

function createFixtureTasks(fixture: SidebarViewConformanceFixture): TaskSwitcherItem[] {
  return Array.from({ length: fixture.source_count }, (_, index) => ({
    id: `${fixture.task_id_prefix}${String(index).padStart(3, "0")}`,
    title:
      index === fixture.matching_task_index
        ? fixture.matching_title
        : `${fixture.title_prefix} ${String(index).padStart(3, "0")}`,
    createdAt: new Date(Date.UTC(2026, 8, 26, 12, index)).toISOString(),
    updatedAt: new Date(Date.UTC(2026, 8, 26, 12, index)).toISOString(),
  }));
}

function viewForFixture(fixture: SidebarViewConformanceFixture): SidebarView {
  return {
    ...DEFAULT_VIEW,
    filters: fixture.query.filters.map((filter, index) => ({ ...filter, id: `fixture-${index}` })),
    sort: fixture.query.sort,
    group: fixture.query.group,
  };
}

describe("sidebar view cross-language conformance", () => {
  it.each(fixtures)("$name matches the complete source fixture", (fixture) => {
    const tasks = createFixtureTasks(fixture);
    const view = viewForFixture(fixture);
    const completeResult = applyView(tasks, view).groups.flatMap((group) =>
      group.tasks.map((task) => task.id),
    );
    const firstSourcePageResult = applyView(tasks.slice(0, 100), view).groups.flatMap((group) =>
      group.tasks.map((task) => task.id),
    );

    expect(completeResult).toEqual(fixture.expected_task_ids);
    expect(firstSourcePageResult).not.toEqual(fixture.expected_task_ids);
  });
});
