import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { TaskMR } from "@/lib/types/gitlab";
import { MRTaskIcon } from "./mr-task-icon";

afterEach(() => cleanup());

const BADGE_TEST_ID = "mr-task-icon-task-1";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "id",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 1,
    mr_url: "",
    mr_title: "Test MR",
    head_branch: "feature",
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

// Seeds the store and renders MRTaskIcon inside the same provider tree in one
// pass — each StateProvider mount owns an independent store instance, so a
// separate seed-then-render step would seed a store the render never reads.
function renderMRTaskIcon(mrs: TaskMR[]) {
  function Fixture() {
    const setTaskMRs = useAppStore((s) => s.setTaskMRs);
    setTaskMRs("ws-1", { "task-1": mrs });
    return <MRTaskIcon taskId="task-1" />;
  }
  return render(
    <TooltipProvider>
      <StateProvider initialState={{ workspaces: { items: [], activeId: "ws-1" } }}>
        <Fixture />
      </StateProvider>
    </TooltipProvider>,
  );
}

describe("MRTaskIcon", () => {
  it("AC27: renders a single-MR badge with data-mr-count=1 and the matching state", () => {
    renderMRTaskIcon([makeMR({ state: "open" })]);
    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.getAttribute("data-mr-count")).toBe("1");
    expect(badge.getAttribute("data-mr-state")).toBe("open");
  });

  it("AC28: renders a multi-MR badge with data-mr-count=N", () => {
    renderMRTaskIcon([makeMR({ id: "a" }), makeMR({ id: "b", mr_iid: 2 })]);
    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.getAttribute("data-mr-count")).toBe("2");
  });

  it("AC29: renders nothing when no MR is linked", () => {
    renderMRTaskIcon([]);
    expect(screen.queryByTestId(BADGE_TEST_ID)).toBeNull();
  });

  it("AC37: a merged MR carries data-mr-state so colour is never the only carrier of state", () => {
    renderMRTaskIcon([makeMR({ state: "merged" })]);
    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.getAttribute("data-mr-state")).toBe("merged");
    // The tooltip text itself (getMRTooltip's "State: merged" output) is
    // covered directly in mr-task-icon.test.ts — jsdom does not reliably
    // open Radix Tooltip content on a non-focusable trigger span.
  });
});
