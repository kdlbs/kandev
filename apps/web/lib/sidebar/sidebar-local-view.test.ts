import { expect, it } from "vitest";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { SidebarTaskQuery } from "@/lib/types/http";
import { localSidebarPage } from "./sidebar-local-view";
import type { LocalSidebarTask } from "./sidebar-local-projection";
import { matchesSidebarClause } from "./sidebar-local-filter";

const query: SidebarTaskQuery = {
  filters: [],
  sort: { key: "title", direction: "asc" },
  group: "none",
  collapsed_group_keys: [],
  collapsed_task_ids: [],
  page: 1,
  page_size: 100,
  locale: "en",
};
const prefs = { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} };
function task(id: string, patch: Partial<LocalSidebarTask> = {}): LocalSidebarTask {
  return {
    id,
    title: id,
    state: "TODO",
    updatedAt: "2026-09-29T00:00:00Z",
    createdAt: "2026-09-29T00:00:00Z",
    overview: toKanbanTask({ id, title: id, state: "TODO" }),
    ...patch,
  };
}
const ids = (page: ReturnType<typeof localSidebarPage>) =>
  page.entries.flatMap((row) => (row.task_id ? [row.task_id] : []));

it.each([0, 100, 101])("bounds a complete %i task inventory", (count) => {
  const tasks = Array.from({ length: count }, (_, index) => task(String(index).padStart(3, "0")));
  const first = localSidebarPage(tasks, query, prefs);
  expect(ids(first)).toHaveLength(Math.min(count, 100));
  expect(first.total_visible_tasks).toBe(count);
  expect(first.has_next).toBe(count > 100);
  const last = localSidebarPage(tasks, { ...query, page: 999 }, prefs);
  expect(last.page).toBe(count > 100 ? 2 : 1);
  if (count === 101) expect(ids(last)).toEqual(["100"]);
});
it("orders ASCII case ties canonically and Unicode by SQLite UTF-8 collation", () => {
  const tasks = [
    task("z", { title: "Alpha" }),
    task("a", { title: "alpha" }),
    task("u", { title: "Ä" }),
    task("l", { title: "ä" }),
    task("astral", { title: "😀" }),
    task("bmp", { title: "\ufffd" }),
  ];
  expect(ids(localSidebarPage(tasks, query, prefs))).toEqual(["a", "z", "u", "l", "bmp", "astral"]);
  expect(matchesSidebarClause("Ä", { dimension: "titleMatch", op: "matches", value: "ä" })).toBe(
    false,
  );
  expect(
    matchesSidebarClause("a_%\\b", { dimension: "titleMatch", op: "matches", value: "_%\\" }),
  ).toBe(true);
});
it("filters before paging and promotes children whose parent does not match", () => {
  const tasks = [task("parent"), task("child", { title: "Needle", parentTaskId: "parent" })];
  const page = localSidebarPage(
    tasks,
    { ...query, filters: [{ dimension: "titleMatch", op: "matches", value: "needle" }] },
    prefs,
  );
  expect(ids(page)).toEqual(["child"]);
  expect(page.entries.find((row) => row.kind === "task")).toMatchObject({
    depth: 0,
    parent_id: undefined,
  });
});
it("breaks cycles deterministically, keeps tree activity, and emits continuation context", () => {
  const tasks = [
    task("b", { parentTaskId: "a", lastActivityAt: "2026-09-29T00:00:00.000000002Z" }),
    task("a", { parentTaskId: "b" }),
    task("c", { lastActivityAt: "2026-09-29T00:00:00.000000001Z" }),
  ];
  const first = localSidebarPage(
    tasks,
    { ...query, sort: { key: "lastActivityAt", direction: "desc" }, page_size: 1 },
    prefs,
  );
  expect(ids(first)).toEqual(["a"]);
  const second = localSidebarPage(
    tasks,
    { ...query, sort: { key: "lastActivityAt", direction: "desc" }, page_size: 1, page: 2 },
    prefs,
  );
  expect(ids(second)).toEqual(["b"]);
  expect(second.entries).toContainEqual({
    kind: "continuation",
    parent_id: "a",
    parent_title: "a",
    depth: 0,
  });
  expect(tasks[1].parentTaskId).toBe("b");
});
it("applies running and activity criteria lexicographically before local paging", () => {
  const tasks = [
    task("waiting-new", {
      sessionState: "WAITING_FOR_INPUT",
      lastActivityAt: "2026-10-05T00:00:00Z",
    }),
    task("running-parent", { lastActivityAt: "2026-10-01T00:00:00Z" }),
    task("running-child", {
      parentTaskId: "running-parent",
      sessionState: "RUNNING",
      lastActivityAt: "2026-10-04T00:00:00Z",
    }),
    task("running-root", {
      sessionState: "RUNNING",
      lastActivityAt: "2026-10-03T00:00:00Z",
    }),
    task("workflow-only", {
      state: "IN_PROGRESS",
      sessionState: "IDLE",
      lastActivityAt: "2026-10-02T00:00:00Z",
    }),
  ];

  const page = localSidebarPage(
    tasks,
    {
      ...query,
      sort: {
        key: "running",
        direction: "desc",
        then_by: [{ key: "lastActivityAt", direction: "desc" }],
      },
    },
    prefs,
  );

  expect(ids(page)).toEqual([
    "running-parent",
    "running-child",
    "running-root",
    "waiting-new",
    "workflow-only",
  ]);
});

it("ranks task-wide running evidence before primary-only legacy state", () => {
  const tasks = [
    task("running-secondary", {
      sessionState: "WAITING_FOR_INPUT",
      hasRunningSession: true,
      lastActivityAt: "2026-10-01T00:00:00Z",
    }),
    task("stale-primary", {
      sessionState: "RUNNING",
      hasRunningSession: false,
      lastActivityAt: "2026-10-05T00:00:00Z",
    }),
    task("legacy-primary", {
      sessionState: "RUNNING",
      lastActivityAt: "2026-10-04T00:00:00Z",
    }),
  ];
  const page = localSidebarPage(
    tasks,
    { ...query, sort: { key: "running", direction: "desc" } },
    prefs,
  );

  expect(ids(page)).toEqual(["legacy-primary", "running-secondary", "stale-primary"]);
});
it("applies root pins, child manual order and collapse before page selection", () => {
  const tasks = [
    task("a"),
    task("b"),
    task("child-a", { parentTaskId: "b" }),
    task("child-z", { parentTaskId: "b" }),
  ];
  const preferences = {
    pinnedTaskIds: ["b"],
    orderedTaskIds: [],
    subtaskOrderByParentId: { b: ["child-z"] },
  };
  expect(ids(localSidebarPage(tasks, query, preferences))).toEqual([
    "b",
    "child-z",
    "child-a",
    "a",
  ]);
  const collapsed = localSidebarPage(tasks, { ...query, collapsed_task_ids: ["b"] }, preferences);
  expect(ids(collapsed)).toEqual(["b", "a"]);
  expect(collapsed.entries.find((row) => row.task_id === "b")?.subtask_count).toBe(2);
  const hidden = localSidebarPage(tasks, { ...query, collapsed_group_keys: ["__all__"] }, prefs);
  expect(hidden.total_entries).toBe(1);
  expect(hidden.entries[0].matching_count).toBe(4);
});
it("keeps a deep tree iterative", () => {
  const tasks = Array.from({ length: 2000 }, (_, index) =>
    task(String(index), { parentTaskId: index ? String(index - 1) : undefined }),
  );
  expect(
    localSidebarPage(tasks, { ...query, page: 20 }, prefs).entries.filter(
      (entry) => entry.kind === "task",
    ),
  ).toHaveLength(100);
});
