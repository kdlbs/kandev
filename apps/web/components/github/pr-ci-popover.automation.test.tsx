import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { cleanup, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

const hookMocks = vi.hoisted(() => ({
  error: null as string | null,
  options: null as TaskCIAutomationOptions | null,
  refreshMock: vi.fn(),
  updateMock: vi.fn(),
  resetPromptMock: vi.fn(),
}));

const responsiveMock = vi.hoisted(() => ({
  isFinePointer: true,
  isMobile: false,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isFinePointer: responsiveMock.isFinePointer,
    isMobile: responsiveMock.isMobile,
  }),
}));

vi.mock("@/hooks/domains/github/use-pr-ci-popover", () => ({
  usePRFeedbackBackgroundSync: vi.fn(),
  usePRCIPopover: () => ({
    feedback: null,
    isFetching: false,
    lastUpdatedAt: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/github/use-task-ci-options", () => ({
  useTaskCIAutomationOptions: () => ({
    options: hookMocks.options ?? makeOptions(),
    loading: false,
    saving: false,
    error: hookMocks.error,
    refresh: hookMocks.refreshMock,
    update: hookMocks.updateMock,
    resetPrompt: hookMocks.resetPromptMock,
  }),
}));

import { PRCIPopover } from "./pr-ci-popover";
import { MultiPRCIPopover } from "./multi-pr-ci-popover";

const MERGED_PROMPT_LABEL = "PR merged";
const CLOSED_PROMPT_LABEL = "PR closed without merging";
const REVIEW_REQUEST_PROMPT_LABEL = "Your review is requested";
const REVIEW_FOLLOW_UP_TRIGGER = "ci-review-follow-up-trigger";
const REMOVE_FIRST_PR_LABEL = "Remove r #1 from task";
const BACKEND_UNAVAILABLE = "backend unavailable";

function makeOptions(overrides: Partial<TaskCIAutomationOptions> = {}): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default CI fix prompt",
    using_default_prompt: true,
    updated_at: "2026-06-18T10:00:00Z",
    pr_states: [],
    ...overrides,
  };
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task-1",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "https://github.com/o/r/pull/1",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "failure",
    mergeable_state: "blocked",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 1,
    checks_total: 2,
    checks_passing: 1,
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

function resetHookMocks() {
  hookMocks.error = null;
  hookMocks.options = null;
  hookMocks.refreshMock.mockReset();
  hookMocks.updateMock.mockReset();
  hookMocks.resetPromptMock.mockReset();
  responsiveMock.isFinePointer = true;
  responsiveMock.isMobile = false;
}

describe("PRCIPopover automation toggles", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders explanatory info and toggles task automation options", async () => {
    renderPopover();

    expect(screen.getByLabelText("Explain CI automation options")).not.toBeNull();
    fireEvent.click(screen.getByLabelText("Auto-fix CI and address comments"));
    fireEvent.click(screen.getByLabelText("Auto-merge when ready"));
    fireEvent.click(screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER));
    fireEvent.click(screen.getByLabelText(REVIEW_REQUEST_PROMPT_LABEL));
    fireEvent.click(screen.getByLabelText(MERGED_PROMPT_LABEL));
    fireEvent.click(screen.getByLabelText(CLOSED_PROMPT_LABEL));

    expect(hookMocks.updateMock).toHaveBeenCalledWith({ auto_fix_enabled: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ auto_merge_enabled: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ prompt_on_review_requested: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ prompt_on_merged: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ prompt_on_closed: true });
  });

  it("keeps review follow-up collapsed until opened while auto-fix and auto-merge stay primary", () => {
    renderPopover();
    const trigger = screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER);

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByLabelText("Auto-fix CI and address comments")).not.toBeNull();
    expect(screen.getByLabelText("Auto-merge when ready")).not.toBeNull();
    expect(screen.queryByLabelText(REVIEW_REQUEST_PROMPT_LABEL)).toBeNull();
    expect(screen.queryByLabelText(MERGED_PROMPT_LABEL)).toBeNull();
    expect(screen.queryByLabelText(CLOSED_PROMPT_LABEL)).toBeNull();

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText(REVIEW_REQUEST_PROMPT_LABEL)).not.toBeNull();
    expect(screen.getByLabelText(MERGED_PROMPT_LABEL)).not.toBeNull();
    expect(screen.getByLabelText(CLOSED_PROMPT_LABEL)).not.toBeNull();
    expect(
      screen.getByLabelText(REVIEW_REQUEST_PROMPT_LABEL).getAttribute("aria-describedby"),
    ).toBe("task-pr-review-requested-prompt-task-1-description");
    expect(screen.getByLabelText(MERGED_PROMPT_LABEL).getAttribute("aria-describedby")).toBe(
      "task-pr-terminal-help-task-1",
    );
    expect(screen.getByLabelText(CLOSED_PROMPT_LABEL).getAttribute("aria-describedby")).toBe(
      "task-pr-terminal-help-task-1",
    );
    expect(
      screen.getByText("Wake the agent for any new request, including re-review after changes."),
    ).not.toBeNull();
    expect(
      screen.getByText("Wake the agent when review work ends. Choose either or both outcomes."),
    ).not.toBeNull();
    expect(screen.getByTestId("ci-review-requested-help")).not.toBeNull();
    expect(screen.getByTestId("ci-pr-terminal-help")).not.toBeNull();
  });

  it("opens review follow-up when lifecycle automation is enabled", () => {
    hookMocks.options = makeOptions({ prompt_on_merged: true });
    renderPopover();

    expect(screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER).getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText(MERGED_PROMPT_LABEL)).not.toBeNull();
  });

  it("keeps the review follow-up trigger compact for desktop and touch-sized otherwise", () => {
    renderPopover();
    const desktopTrigger = screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER);
    expect(desktopTrigger.classList.contains("min-h-7")).toBe(true);
    expect(desktopTrigger.classList.contains("min-h-11")).toBe(false);

    cleanup();
    responsiveMock.isMobile = true;
    renderPopover();
    expect(screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER).classList.contains("min-h-11")).toBe(true);

    cleanup();
    responsiveMock.isMobile = false;
    responsiveMock.isFinePointer = false;
    renderPopover();
    expect(screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER).classList.contains("min-h-11")).toBe(true);
  });
});

describe("PRCIPopover automation status", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the connected-account review label and selected PR error", () => {
    hookMocks.options = makeOptions({
      pr_states: [
        {
          task_id: "task-1",
          repository_id: "",
          pr_number: 1,
          last_fix_signature: "",
          last_fix_checkpoint_json: "",
          last_fix_enqueued_at: null,
          last_fix_session_id: null,
          auto_fix_round_count: 0,
          auto_fix_exhausted_at: null,
          last_merge_signature: "",
          last_merge_attempt_at: null,
          last_error: "No promptable task session is available.",
          created_at: "",
          updated_at: "",
        },
      ],
    });
    renderPopover();

    expect(screen.getByRole("alert").textContent).toContain(
      "No promptable task session is available.",
    );
    fireEvent.click(screen.getByTestId(REVIEW_FOLLOW_UP_TRIGGER));
    expect(screen.getByLabelText(REVIEW_REQUEST_PROMPT_LABEL)).not.toBeNull();
    expect(screen.queryByLabelText(/edit.*review/i)).toBeNull();
  });

  it("uses the PR title in the header and omits the redundant detail link", () => {
    renderPopover();

    expect(screen.getByTestId("pr-popover-title").textContent).toBe("#1 Test PR");
    expect(screen.queryByText("CI status")).toBeNull();
    expect(screen.queryByText("Open PR details")).toBeNull();
    expect(screen.queryByLabelText("View all checks on GitHub")).toBeNull();
    expect(screen.getByLabelText("View pull request on GitHub")).not.toBeNull();
  });

  it("opens the in-app detail panel from the selected multi-PR title", () => {
    const onOpenDetailPanel = vi.fn();
    render(
      <TooltipProvider>
        <StateProvider>
          <ToastProvider>
            <MultiPRCIPopover
              prs={[
                makePR({ id: "a", pr_number: 1, pr_title: "First PR", checks_state: "success" }),
                makePR({ id: "b", pr_number: 2, pr_title: "Second PR" }),
              ]}
              enabled={true}
              onOpenDetailPanel={onOpenDetailPanel}
            />
          </ToastProvider>
        </StateProvider>
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open #2 Second PR details" }));

    expect(onOpenDetailPanel).toHaveBeenCalledTimes(1);
    expect(onOpenDetailPanel).toHaveBeenCalledWith(expect.objectContaining({ id: "b" }));
    expect(screen.queryByText("Open PR details")).toBeNull();
  });
});

describe("MultiPRCIPopover unlink", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("unlinks one multi-PR tab while keeping its sibling visible", async () => {
    const onCollapseFocus = vi.fn();
    function RemovablePopover() {
      const [prs, setPrs] = useState([
        makePR({ id: "a", pr_number: 1, pr_title: "First PR" }),
        makePR({ id: "b", pr_number: 2, pr_title: "Second PR", checks_state: "success" }),
      ]);
      return (
        <MultiPRCIPopover
          prs={prs}
          enabled={true}
          onRemovePR={async (pr) =>
            setPrs((current) => current.filter((item) => item.id !== pr.id))
          }
          onCollapseFocus={onCollapseFocus}
        />
      );
    }

    render(
      <TooltipProvider>
        <StateProvider>
          <ToastProvider>
            <RemovablePopover />
          </ToastProvider>
        </StateProvider>
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: REMOVE_FIRST_PR_LABEL }));

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: REMOVE_FIRST_PR_LABEL })).toBeNull(),
    );
    expect(screen.getByRole("tab", { name: "r #2" })).not.toBeNull();
    expect(onCollapseFocus).toHaveBeenCalledWith(expect.objectContaining({ id: "b" }));
  });

  it("keeps a failed unlink tab and reports the error", async () => {
    const onRemovePR = vi.fn().mockRejectedValue(new Error(BACKEND_UNAVAILABLE));
    render(
      <TooltipProvider>
        <StateProvider>
          <ToastProvider>
            <MultiPRCIPopover
              prs={[makePR({ id: "a", pr_number: 1 }), makePR({ id: "b", pr_number: 2 })]}
              enabled={true}
              onRemovePR={onRemovePR}
            />
          </ToastProvider>
        </StateProvider>
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: REMOVE_FIRST_PR_LABEL }));

    await waitFor(() =>
      expect(screen.getByTestId("toast-message").textContent).toContain(BACKEND_UNAVAILABLE),
    );
    expect(screen.getByRole("button", { name: REMOVE_FIRST_PR_LABEL })).not.toBeNull();
    expect(onRemovePR).toHaveBeenCalledWith(expect.objectContaining({ id: "a" }));
  });

  it("focuses the next adjacent tab when a selected tab is removed from a larger set", async () => {
    function RemovablePopover() {
      const [prs, setPrs] = useState([
        makePR({ id: "a", pr_number: 1, pr_title: "First PR", checks_state: "success" }),
        makePR({ id: "b", pr_number: 2, pr_title: "Second PR" }),
        makePR({ id: "c", pr_number: 3, pr_title: "Third PR", checks_state: "success" }),
      ]);
      return (
        <MultiPRCIPopover
          prs={prs}
          enabled={true}
          onRemovePR={async (pr) =>
            setPrs((current) => current.filter((item) => item.id !== pr.id))
          }
        />
      );
    }

    render(
      <TooltipProvider>
        <StateProvider>
          <ToastProvider>
            <RemovablePopover />
          </ToastProvider>
        </StateProvider>
      </TooltipProvider>,
    );

    expect(screen.getByRole("tab", { name: "r #2" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Remove r #2 from task" }));

    await waitFor(() => expect(screen.queryByRole("tab", { name: "r #2" })).toBeNull());
    const nextTab = screen.getByRole("tab", { name: "r #3" });
    expect(nextTab.getAttribute("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(nextTab);
  });
});

describe("PRCIPopover task prompts", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("opens a task prompt dialog with a settings link and saves overrides", async () => {
    renderPopover();

    fireEvent.click(screen.getByLabelText("Edit auto-fix prompt for this task"));

    expect(screen.getByRole("dialog").textContent).toContain("Auto-fix prompt");
    expect(screen.getByRole("link", { name: "Edit default prompt" }).getAttribute("href")).toBe(
      "/settings/prompts",
    );
    expect(screen.getByRole("dialog").textContent).toContain("{{pr.feedback}}");
    expect(screen.getByRole("dialog").textContent).toContain("new or changed failing checks");
    expect(screen.getByRole("dialog").textContent).toContain("review comments");

    const textarea = screen.getByLabelText("Task auto-fix prompt");
    fireEvent.click(screen.getByRole("button", { name: "Insert PR feedback" }));
    expect((textarea as HTMLTextAreaElement).value).toContain("{{pr.feedback}}");
    fireEvent.change(textarea, { target: { value: "Please fix this PR." } });
    fireEvent.click(screen.getByRole("button", { name: "Save prompt" }));

    await waitFor(() => {
      expect(hookMocks.updateMock).toHaveBeenCalledWith({
        auto_fix_prompt_override: "Please fix this PR.",
      });
    });
  });

  it("uses the default prompt when requested", async () => {
    renderPopover();

    fireEvent.click(screen.getByLabelText("Edit auto-fix prompt for this task"));
    fireEvent.click(screen.getByRole("button", { name: "Use default" }));

    await waitFor(() => {
      expect(hookMocks.resetPromptMock).toHaveBeenCalledTimes(1);
    });
    expect(hookMocks.updateMock).not.toHaveBeenCalled();
  });

  it("offers retry after CI automation options fail to load", () => {
    hookMocks.error = BACKEND_UNAVAILABLE;
    renderPopover();

    expect(screen.getByText(BACKEND_UNAVAILABLE)).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(hookMocks.refreshMock).toHaveBeenCalledTimes(1);
  });
});
