import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { PRFeedback, TaskPR } from "@/lib/types/github";

const feedbackState = vi.hoisted(() => ({ value: null as PRFeedback | null }));

vi.mock("@/hooks/domains/github/use-pr-ci-popover", () => ({
  usePRFeedbackBackgroundSync: vi.fn(),
  usePRCIPopover: () => ({
    feedback: feedbackState.value,
    isFetching: false,
    isRefreshing: false,
    lastUpdatedAt: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: { authenticated: true } }),
}));

vi.mock("@/hooks/domains/github/use-task-ci-options", () => ({
  useTaskCIAutomationOptions: () => ({
    options: null,
    loading: false,
    saving: false,
    error: null,
    refresh: vi.fn(),
    update: vi.fn(),
    resetPrompt: vi.fn(),
  }),
}));

import { PRCIPopover } from "./pr-ci-popover";

function makePR(): TaskPR {
  return {
    id: "id",
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "demo",
    pr_number: 42,
    pr_url: "https://github.com/acme/demo/pull/42",
    pr_title: "Current checks",
    head_branch: "feature",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "failure",
    mergeable_state: "blocked",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
  };
}

function renderPopover() {
  return render(
    <TooltipProvider>
      <StateProvider>
        <ToastProvider>
          <PRCIPopover pr={makePR()} enabled={true} />
        </ToastProvider>
      </StateProvider>
    </TooltipProvider>,
  );
}

describe("PR check disclosure cancellation", () => {
  beforeEach(() => {
    feedbackState.value = {
      pr: {} as PRFeedback["pr"],
      reviews: [],
      comments: [],
      checks: [
        {
          name: "Preview / deploy-fork",
          source: "check_run",
          status: "completed",
          conclusion: "cancelled",
          html_url: "https://example.test/checks/deploy",
          output: "",
          started_at: null,
          completed_at: null,
        },
      ],
      checks_state: "",
      has_issues: false,
    };
  });

  afterEach(() => {
    cleanup();
    feedbackState.value = null;
  });

  it("shows non-success empty copy without a failure action or fake pass state", () => {
    renderPopover();

    expect(screen.getByTestId("pr-checks-empty").textContent).toBe("Checks not successful");
    expect(screen.queryByText("No checks have started")).toBeNull();
    expect(screen.queryByTestId("pr-check-group")).toBeNull();
    expect(screen.queryByTestId("pr-workflow-add-context")).toBeNull();
  });

  it("uses an authoritative empty check list instead of stale aggregate counts", () => {
    feedbackState.value = {
      ...feedbackState.value!,
      checks: [],
      checks_state: "pending",
    };
    renderPopover();

    expect(screen.getByTestId("pr-checks-empty").textContent).toBe("No checks have started");
    expect(screen.queryByTestId("pr-check-group")).toBeNull();
  });
});
