import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { TaskMR } from "@/lib/types/gitlab";

// AC5's load-bearing clause is that MRTaskIcon is never invoked when the row
// has no taskId — a plain stub (as used elsewhere for module-level
// interception) records no calls, so this needs its own vi.fn()-backed mock.
// Isolated in its own file so the module mock doesn't shadow the real
// MRTaskIcon rendering asserted by the other task-item scenarios.
vi.mock("@/components/gitlab/mr-task-icon", () => ({
  MRTaskIcon: vi.fn(() => null),
}));

import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { TaskItem } from "./task-item";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "mr-1",
    task_id: "other-task",
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
    ...overrides,
  };
}

afterEach(() => cleanup());

describe("TaskItem, no taskId (AC5)", () => {
  it("renders no mr-task-icon element and never invokes MRTaskIcon", () => {
    render(
      <StateProvider
        initialState={{
          taskMRs: { byWorkspaceId: { ws1: { "other-task": [makeMR()] } } },
          workspaces: { items: [], activeId: "ws1" },
        }}
      >
        <TooltipProvider>
          <TaskItem title="Needs answer" state="REVIEW" />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(screen.queryByTestId(/^mr-task-icon-/)).toBeNull();
    expect(MRTaskIcon).not.toHaveBeenCalled();
  });
});
