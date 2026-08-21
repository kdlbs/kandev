import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
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

// Seeds the store via StateProvider's initialState rather than a render-time
// setTaskMRs call, matching the pattern task-title-hover-card.test.tsx uses.
function renderMRTaskIcon(mrs: TaskMR[]) {
  return render(
    <TooltipProvider>
      <StateProvider
        initialState={{
          workspaces: { items: [], activeId: "ws-1" },
          taskMRs: { byWorkspaceId: { "ws-1": { "task-1": mrs } } },
        }}
      >
        <MRTaskIcon taskId="task-1" />
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
  });

  // AC1, AC7, AC24: the trigger is keyboard-focusable and accessible, and the
  // rendered summary uses MR !{iid} plus labelled rows — no pipe-delimited
  // string anywhere.
  it("AC1/AC7: trigger has role=img, tabIndex=0, and a localized aria-label", () => {
    renderMRTaskIcon([makeMR({ mr_iid: 42 })]);
    const badge = screen.getByTestId(BADGE_TEST_ID);
    expect(badge.getAttribute("role")).toBe("img");
    expect(badge.getAttribute("tabIndex")).toBe("0");
    expect(badge.getAttribute("aria-label")).toBe("Merge request !42 status");
  });

  it("AC1/AC24: hovering the trigger opens the structured summary with MR !iid and the title", () => {
    renderMRTaskIcon([
      makeMR({
        mr_iid: 7,
        mr_title: "Fix the login flow",
        approval_state: "approved",
        pipeline_state: "success",
        detailed_merge_status: "mergeable",
      }),
    ]);
    const badge = screen.getByTestId(BADGE_TEST_ID);
    fireEvent.pointerEnter(badge, { pointerType: "mouse" });

    // Radix mounts the tooltip content into both a measurement copy and the
    // visible portal, so scope to the first (portal) match rather than
    // asserting a single element.
    expect(screen.getAllByTestId("mr-task-status-summary").length).toBeGreaterThan(0);
    expect(screen.getAllByTestId("mr-task-status-iid")[0].textContent).toBe("MR !7");
    expect(screen.getAllByTestId("mr-task-status-title")[0].textContent).toBe("Fix the login flow");
    expect(screen.getAllByTestId("mr-task-status-review")[0].textContent).toContain("Review");
    expect(screen.getAllByTestId("mr-task-status-review")[0].textContent).toContain("Approved");
    expect(screen.getAllByTestId("mr-task-status-ci")[0].textContent).toContain("CI");
    expect(screen.getAllByTestId("mr-task-status-merge")[0].textContent).toContain("Merge");
    expect(document.body.textContent).not.toContain("|");
  });

  it("AC1/AC24: the multi-MR badge renders one summary entry per MR, separated", () => {
    renderMRTaskIcon([
      makeMR({ id: "a", mr_iid: 11, mr_title: "First linked MR", pipeline_state: "success" }),
      makeMR({ id: "b", mr_iid: 12, mr_title: "Second linked MR", pipeline_state: "failure" }),
    ]);
    fireEvent.pointerEnter(screen.getByTestId(BADGE_TEST_ID), { pointerType: "mouse" });

    // Radix force-mounts a measurement copy alongside the visible portal, so
    // scope to one summary rather than counting matches across the document.
    const summary = screen.getAllByTestId("mr-task-status-summary")[0];
    const entries = summary.querySelectorAll('[data-testid="mr-task-status-entry"]');
    expect(entries).toHaveLength(2);
    expect(entries[0].textContent).toContain("MR !11");
    expect(entries[0].textContent).toContain("First linked MR");
    expect(entries[0].textContent).toContain("Passed");
    expect(entries[1].textContent).toContain("MR !12");
    expect(entries[1].textContent).toContain("Second linked MR");
    expect(entries[1].textContent).toContain("Failed");
    // Only the trailing entries carry the divider.
    expect(entries[0].className).not.toContain("border-t");
    expect(entries[1].className).toContain("border-t");
  });

  it("AC7: the multi-MR badge reports ready-to-merge only when every open MR is ready", () => {
    const ready = {
      pipeline_state: "success" as const,
      detailed_merge_status: "mergeable",
    };
    renderMRTaskIcon([makeMR({ id: "a", ...ready }), makeMR({ id: "b", mr_iid: 2, ...ready })]);
    expect(screen.getByTestId(BADGE_TEST_ID).getAttribute("data-mr-ready-to-merge")).toBe("true");

    cleanup();
    renderMRTaskIcon([
      makeMR({ id: "a", ...ready }),
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }),
    ]);
    expect(screen.getByTestId(BADGE_TEST_ID).getAttribute("data-mr-ready-to-merge")).toBe("false");
  });

  it("malformed store value still bails to null instead of throwing", () => {
    render(
      <TooltipProvider>
        <StateProvider
          initialState={{
            workspaces: { items: [], activeId: "ws-1" },
            // @ts-expect-error simulating a partial hydration payload
            taskMRs: { byWorkspaceId: { "ws-1": { "task-1": { not: "an array" } } } },
          }}
        >
          <MRTaskIcon taskId="task-1" />
        </StateProvider>
      </TooltipProvider>,
    );
    expect(screen.queryByTestId(BADGE_TEST_ID)).toBeNull();
  });
});
