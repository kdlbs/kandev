import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { PRFeedback, TaskPR } from "@/lib/types/github";

const feedbackState = vi.hoisted(() => ({
  value: null as PRFeedback | null,
  loading: false,
  authenticated: true,
}));

vi.mock("@/hooks/domains/github/use-pr-ci-popover", () => ({
  usePRFeedbackBackgroundSync: vi.fn(),
  usePRCIPopover: () => ({
    feedback: feedbackState.value,
    isFetching: feedbackState.loading,
    isRefreshing: feedbackState.loading,
    lastUpdatedAt: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: { authenticated: feedbackState.authenticated } }),
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

const EMPTY_CHECKS_TEST_ID = "pr-checks-empty";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
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
    ...overrides,
  };
}

function renderPopover(overrides: Partial<TaskPR> = {}) {
  return render(
    <TooltipProvider>
      <StateProvider>
        <ToastProvider>
          <PRCIPopover pr={makePR(overrides)} enabled={true} />
        </ToastProvider>
      </StateProvider>
    </TooltipProvider>,
  );
}

describe("PR check disclosure cancellation", () => {
  beforeEach(() => {
    feedbackState.loading = false;
    feedbackState.authenticated = true;
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

    expect(screen.getByTestId(EMPTY_CHECKS_TEST_ID).textContent).toBe("Checks not successful");
    expect(screen.queryByText("No checks have started")).toBeNull();
    expect(screen.queryByTestId("pr-check-group")).toBeNull();
    expect(screen.queryByTestId("pr-workflow-add-context")).toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.2/.3
  it("keeps the PR action available through initial loading and authentication failure", () => {
    feedbackState.value = null;
    feedbackState.loading = true;
    const first = renderPopover();
    const link = screen.getByRole("link", { name: "Open PR" });
    expect(link.textContent).toContain("Open PR");
    expect(link.getAttribute("href")).toBe(makePR().pr_url);
    expect(screen.getByRole("status").textContent).toContain("Updating");
    first.unmount();
    feedbackState.authenticated = false;
    renderPopover();
    expect(screen.getByRole("link", { name: "Open PR" }).getAttribute("href")).toBe(
      makePR().pr_url,
    );
    expect(screen.getByTestId("pr-popover-reconnect-link")).toBeTruthy();
  });

  it("retains cached check results while refreshing", () => {
    feedbackState.loading = true;
    renderPopover();
    expect(screen.getByTestId(EMPTY_CHECKS_TEST_ID).textContent).toBe("Checks not successful");
    expect(screen.queryByTestId("pr-checks-loading")).toBeNull();
    expect(screen.getByRole("status").textContent).toContain("Updating");
  });

  it("uses an authoritative empty check list instead of stale aggregate counts", () => {
    feedbackState.value = {
      ...feedbackState.value!,
      checks: [],
      checks_state: "pending",
    };
    renderPopover();

    expect(screen.getByTestId(EMPTY_CHECKS_TEST_ID).textContent).toBe("No checks have started");
    expect(screen.queryByTestId("pr-check-group")).toBeNull();
  });
});

describe("PR check disclosure skipped checks", () => {
  beforeEach(() => {
    feedbackState.loading = false;
    feedbackState.authenticated = true;
  });

  afterEach(() => {
    cleanup();
    feedbackState.value = null;
  });

  function skippedCheck(name: string) {
    return {
      name,
      source: "check_run" as const,
      status: "completed",
      conclusion: "skipped",
      html_url: "",
      output: "",
      started_at: null,
      completed_at: null,
    };
  }

  it("explains an all-skipped loaded snapshot without claiming tests ran", () => {
    feedbackState.value = {
      pr: {} as PRFeedback["pr"],
      reviews: [],
      comments: [],
      checks: [skippedCheck("docs"), skippedCheck("lint")],
      checks_state: "",
      has_issues: false,
    };
    renderPopover({ checks_state: "", checks_total: 0, checks_passing: 0 });

    expect(screen.getByTestId(EMPTY_CHECKS_TEST_ID).textContent).toBe("All checks skipped");
    expect(screen.queryByText("No checks have started")).toBeNull();
  });

  it("uses persisted all-skipped metadata when detailed results are unavailable", () => {
    feedbackState.value = null;
    renderPopover({
      checks_state: "",
      checks_total: 0,
      checks_passing: 0,
      head_sha: "head-a",
      check_summary: { head_sha: "head-a", observed: 2, skipped: 2 },
    });

    expect(screen.getByTestId(EMPTY_CHECKS_TEST_ID).textContent).toBe("All checks skipped");
  });

  it("keeps cancelled-only results distinct from skipped results", () => {
    feedbackState.value = {
      pr: {} as PRFeedback["pr"],
      reviews: [],
      comments: [],
      checks: [
        {
          ...skippedCheck("deploy"),
          conclusion: "cancelled",
        },
      ],
      checks_state: "",
      has_issues: false,
    };
    renderPopover({ checks_state: "", checks_total: 0, checks_passing: 0 });

    expect(screen.getByTestId(EMPTY_CHECKS_TEST_ID).textContent).toBe("Checks not successful");
  });

  it("keeps skipped counts separate from passed counts", () => {
    feedbackState.value = {
      pr: {} as PRFeedback["pr"],
      reviews: [],
      comments: [],
      checks: [
        {
          ...skippedCheck("unit"),
          conclusion: "success",
        },
        skippedCheck("docs"),
      ],
      checks_state: "success",
      has_issues: false,
    };
    renderPopover({ checks_state: "success", checks_total: 1, checks_passing: 1 });

    expect(screen.getByTestId("pr-checks-skipped-count").textContent).toBe("1 check skipped");
    expect(screen.getByTestId("pr-check-group-count").textContent).toBe("1");
  });
});
