import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { TaskPR } from "@/lib/types/github";

const popoverState = vi.hoisted(() => ({
  isFetching: false,
  isRefreshing: false,
  lastUpdatedAt: null as number | null,
}));

vi.mock("@/hooks/domains/github/use-pr-ci-popover", () => ({
  usePRFeedbackBackgroundSync: vi.fn(),
  usePRCIPopover: () => ({
    feedback: null,
    isFetching: popoverState.isFetching,
    isRefreshing: popoverState.isRefreshing,
    lastUpdatedAt: popoverState.lastUpdatedAt,
    refetch: vi.fn(),
  }),
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

const UPDATING_LABEL = "Updating…";

function makePR(): TaskPR {
  return {
    id: "id",
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "app",
    pr_number: 7,
    pr_url: "https://github.com/acme/app/pull/7",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 1,
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

describe("PR popover footer refresh indicator", () => {
  beforeEach(() => {
    popoverState.isFetching = false;
    popoverState.isRefreshing = false;
    popoverState.lastUpdatedAt = null;
  });

  afterEach(() => {
    cleanup();
  });

  it("shows a spinner and 'Updating…' instead of a freshness claim while refreshing", () => {
    popoverState.isRefreshing = true;
    popoverState.lastUpdatedAt = Date.now();
    renderPopover();

    const updating = screen.getByTestId("pr-popover-updating");
    expect(updating.textContent).toContain(UPDATING_LABEL);
    expect(updating.querySelector(".animate-spin")).not.toBeNull();
    expect(screen.queryByTestId("pr-popover-updated-at")).toBeNull();
  });

  it("announces the refresh to assistive tech", () => {
    popoverState.isRefreshing = true;
    popoverState.lastUpdatedAt = Date.now();
    renderPopover();

    expect(screen.getByRole("status").textContent).toContain(UPDATING_LABEL);
  });

  it("shows the elapsed label once the refresh settles", () => {
    popoverState.isRefreshing = false;
    popoverState.lastUpdatedAt = Date.now();
    renderPopover();

    expect(screen.getByTestId("pr-popover-updated-at").textContent).toBe("updated just now");
    expect(screen.queryByTestId("pr-popover-updating")).toBeNull();
  });

  it("shows the indicator on a first open, when there is no cached timestamp yet", () => {
    popoverState.isRefreshing = true;
    popoverState.lastUpdatedAt = null;
    renderPopover();

    expect(screen.getByTestId("pr-popover-updating").textContent).toContain(UPDATING_LABEL);
  });

  it("renders no footer at all when there is nothing to report", () => {
    popoverState.isRefreshing = false;
    popoverState.lastUpdatedAt = null;
    renderPopover();

    expect(screen.queryByTestId("pr-popover-footer")).toBeNull();
  });
});
