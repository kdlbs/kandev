import { expect, it } from "vitest";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { defaultState } from "@/lib/state/default-state";
import type { SidebarTaskQuery } from "@/lib/types/http";
import { applyFilters } from "./apply-view";
import { projectLocalSidebarTasks } from "./sidebar-local-projection";
import { localSidebarPage } from "./sidebar-local-view";

const cases: [Record<string, unknown> | null, boolean, boolean][] = [
  [{ issue_watch_id: "watch" }, true, false],
  [{ review_watch_id: "watch" }, false, true],
  [{ gitlab_issue_watch_id: "watch" }, true, false],
  [{ gitlab_review_watch_id: "watch" }, false, true],
  [{ jira_issue_watch_id: "watch" }, true, false],
  [{ linear_issue_watch_id: "watch" }, true, false],
  [{ sentry_issue_watch_id: "watch" }, true, false],
  [{ azure_devops_work_item_watch_id: "watch" }, true, false],
  [{ azure_devops_pr_watch_id: "watch" }, false, true],
  [null, false, false],
  [
    { issue_url: "https://example.test/issues/1", pr_url: "https://example.test/pulls/2" },
    false,
    false,
  ],
  [{ custom_issue_watch_id: "watch", custom_review_watch_id: "watch" }, false, false],
  [
    {
      issue_watch_id: "",
      gitlab_issue_watch_id: "issue",
      review_watch_id: null,
      azure_devops_pr_watch_id: "review",
    },
    true,
    true,
  ],
];
for (const value of [null, "", "undefined", false, true, 0, 42, ["watch"], { id: "watch" }]) {
  cases.push([
    {
      issue_watch_id: value,
      review_watch_id: value,
      gitlab_issue_watch_id: value,
      gitlab_review_watch_id: value,
    },
    false,
    false,
  ]);
}

// @covers AC-UI-PROVIDER-WATCHER-TYPES-001.1 through .7
it.each(cases)("local and mapped Sidebar filters agree for %j", (metadata, issue, review) => {
  const mapped = toKanbanTask({
    id: "watch-task",
    title: "Watcher task",
    workspace_id: "ws",
    workflow_id: "wf",
    metadata,
  });
  const projected = projectLocalSidebarTasks([mapped], defaultState, "ws");
  for (const [dimension, expected] of [
    ["isIssueWatch", issue],
    ["isPRReview", review],
  ] as const) {
    for (const negative of [false, true]) {
      const filter = {
        id: "watcher",
        dimension,
        op: negative ? ("is_not" as const) : ("is" as const),
        value: true,
      };
      const query: SidebarTaskQuery = {
        filters: [filter],
        sort: { key: "title", direction: "asc" },
        group: "none",
        collapsed_group_keys: [],
        collapsed_task_ids: [],
        page: 1,
        page_size: 100,
        locale: "en",
      };
      const page = localSidebarPage(projected, query, {
        pinnedTaskIds: [],
        orderedTaskIds: [],
        subtaskOrderByParentId: {},
      });
      const ids = expected !== negative ? ["watch-task"] : [];
      expect(page.entries.flatMap((row) => (row.task_id ? [row.task_id] : []))).toEqual(ids);
      expect(page.total_tasks).toBe(ids.length);
      expect(page.total_visible_tasks).toBe(ids.length);
      const { id, title, isIssueWatch, isPRReview } = mapped;
      expect(
        applyFilters([{ id, title, isIssueWatch, isPRReview }], [filter]).map((task) => task.id),
      ).toEqual(ids);
    }
  }
});
