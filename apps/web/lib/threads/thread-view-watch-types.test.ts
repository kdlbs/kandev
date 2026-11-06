import { describe, expect, it } from "vitest";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { ThreadView } from "@/lib/state/slices/ui/thread-view-types";
import { queryThreadView } from "./thread-view-query";

const cases: [string, Record<string, unknown> | null, string][] = [
  ["github-issue", { issue_watch_id: "watch" }, "issue_watch"],
  ["github-review", { review_watch_id: "watch" }, "pull_request_review"],
  ["gitlab-issue", { gitlab_issue_watch_id: "watch" }, "issue_watch"],
  ["gitlab-review", { gitlab_review_watch_id: "watch" }, "pull_request_review"],
  ["jira", { jira_issue_watch_id: "watch" }, "issue_watch"],
  ["linear", { linear_issue_watch_id: "watch" }, "issue_watch"],
  ["sentry", { sentry_issue_watch_id: "watch" }, "issue_watch"],
  ["azure-issue", { azure_devops_work_item_watch_id: "watch" }, "issue_watch"],
  ["azure-review", { azure_devops_pr_watch_id: "watch" }, "pull_request_review"],
  ["ordinary", null, "standard"],
  [
    "linked",
    { issue_url: "https://example.test/issues/1", pr_url: "https://example.test/pulls/2" },
    "standard",
  ],
  ["unknown", { custom_issue_watch_id: "watch", custom_review_watch_id: "watch" }, "standard"],
  [
    "mixed-both",
    {
      issue_watch_id: "",
      gitlab_issue_watch_id: "issue",
      review_watch_id: null,
      gitlab_review_watch_id: "review",
    },
    "pull_request_review",
  ],
];

function filtered(metadata: Record<string, unknown> | null, kind: string, negative = false) {
  const task = toKanbanTask({
    id: "watch-task",
    workspace_id: "ws",
    workflow_id: "wf",
    workflow_step_id: "step",
    title: "Watcher task",
    state: "IN_PROGRESS",
    primary_session_id: "session",
    primary_session_state: "RUNNING",
    metadata,
  });
  const view: ThreadView = {
    id: "view",
    name: "Watchers",
    taskScope: { mode: "all", taskIds: [] },
    filters: [{ id: "type", dimension: "taskType", op: negative ? "is_not" : "is", value: kind }],
    sort: { key: "title", direction: "asc" },
    maxColumns: null,
    layout: "columns",
    autoHideComposer: false,
  };
  return queryThreadView(
    { wf: { workflowId: "wf", workflowName: "Workflow", steps: [], tasks: [task] } },
    view,
    { workspaceId: "ws" },
  );
}

// @covers AC-UI-PROVIDER-WATCHER-TYPES-001.1 through .7
describe("mapped task metadata reaches Threads saved type filters", () => {
  it.each(cases)("%s", (_name, metadata, expected) => {
    for (const kind of ["standard", "issue_watch", "pull_request_review"]) {
      const result = filtered(metadata, kind);
      expect(result.matchingCandidates.map((task) => task.taskId)).toEqual(
        kind === expected ? ["watch-task"] : [],
      );
      expect(result.matchingCount).toBe(kind === expected ? 1 : 0);
      expect(filtered(metadata, kind, true).matchingCount).toBe(kind === expected ? 0 : 1);
    }
  });

  it.each([null, "", "undefined", false, true, 0, 42, ["watch"], { id: "watch" }])(
    "unusable identifiers %j remain ordinary",
    (value) => {
      const metadata = {
        issue_watch_id: value,
        review_watch_id: value,
        gitlab_issue_watch_id: value,
        gitlab_review_watch_id: value,
      };
      expect(filtered(metadata, "standard").matchingCount).toBe(1);
    },
  );
});
