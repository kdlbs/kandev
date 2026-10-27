import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { HydrationState } from "@/lib/state/store";
import type { TaskMR } from "@/lib/types/gitlab";
import { TaskItem } from "./task-item";

const PR_ICON_TEST_ID = "pr-task-icon-t1";
const MR_ICON_TEST_ID = "mr-task-icon-t1";

const mr: TaskMR = {
  id: "mr-1",
  task_id: "t1",
  host: "https://gitlab.com",
  project_path: "acme/api",
  mr_iid: 1,
  mr_url: "",
  mr_title: "Test MR",
  head_branch: "feat",
  base_branch: "main",
  author_username: "alice",
  state: "open",
  approval_state: "",
  pipeline_state: "",
  merge_status: "",
  draft: false,
  approval_count: 0,
  required_approvals: 0,
  pipeline_jobs_total: 0,
  pipeline_jobs_pass: 0,
  reviewer_count: 0,
  unapproved_reviewers: 0,
  unresolved_discussions: 0,
  created_at: "",
  updated_at: "",
};

afterEach(cleanup);

it("hides change-request status for Nothing and restores it when the selection changes", () => {
  const state: HydrationState = {
    taskMRs: { byWorkspaceId: { ws1: { t1: [mr] } } },
    workspaces: { items: [], activeId: "ws1" },
  };
  const renderRow = (
    trailing: "none" | "git_changes" | "relative_time" | "change_request_status",
  ) => (
    <StateProvider initialState={state}>
      <TooltipProvider>
        <TaskItem
          title="Needs answer"
          state="REVIEW"
          taskId="t1"
          priority="high"
          prInfo={{ number: 7, state: "open" }}
          taskRowPresentation={{
            detailsEnabled: true,
            detailOrder: ["relative_time", "repository", "pull_request_number"],
            visibleDetails: ["pull_request_number"],
            trailing,
          }}
        />
      </TooltipProvider>
    </StateProvider>
  );
  const { rerender } = render(renderRow("git_changes"));
  expect(screen.getByTestId(PR_ICON_TEST_ID)).toBeTruthy();
  expect(screen.getByTestId(MR_ICON_TEST_ID)).toBeTruthy();

  rerender(renderRow("none"));
  expect(screen.queryByTestId(PR_ICON_TEST_ID)).toBeNull();
  expect(screen.queryByTestId(MR_ICON_TEST_ID)).toBeNull();
  expect(screen.getByText("#7")).toBeTruthy();
  expect(screen.getByTestId("sidebar-task-priority-indicator")).toBeTruthy();
  expect(screen.getByLabelText("Task actions")).toBeTruthy();

  rerender(renderRow("change_request_status"));
  const trailing = screen.getByTestId("sidebar-task-change-request-actions");
  expect(trailing.contains(screen.getByTestId(PR_ICON_TEST_ID))).toBe(true);
  expect(trailing.contains(screen.getByTestId(MR_ICON_TEST_ID))).toBe(true);

  rerender(renderRow("relative_time"));
  expect(screen.getByTestId(PR_ICON_TEST_ID)).toBeTruthy();
  expect(screen.getByTestId(MR_ICON_TEST_ID)).toBeTruthy();
});
