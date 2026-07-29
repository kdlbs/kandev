import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
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

const MERGED_PROMPT_LABEL = "Prompt agent when PR is merged";

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
    fireEvent.click(screen.getByLabelText("Prompt agent when your review is requested"));
    fireEvent.click(screen.getByLabelText(MERGED_PROMPT_LABEL));
    fireEvent.click(screen.getByLabelText("Prompt agent when PR is closed"));

    expect(hookMocks.updateMock).toHaveBeenCalledWith({ auto_fix_enabled: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ auto_merge_enabled: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ prompt_on_review_requested: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ prompt_on_merged: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ prompt_on_closed: true });
  });

  it("keeps switch rows compact for fine-pointer desktop and touch-sized otherwise", () => {
    renderPopover();
    const desktopRow = screen.getByLabelText(MERGED_PROMPT_LABEL).parentElement;
    expect(desktopRow?.classList.contains("min-h-7")).toBe(true);
    expect(desktopRow?.classList.contains("min-h-11")).toBe(false);

    cleanup();
    responsiveMock.isMobile = true;
    renderPopover();
    const mobileRow = screen.getByLabelText(MERGED_PROMPT_LABEL).parentElement;
    expect(mobileRow?.classList.contains("min-h-11")).toBe(true);

    cleanup();
    responsiveMock.isMobile = false;
    responsiveMock.isFinePointer = false;
    renderPopover();
    const coarsePointerRow = screen.getByLabelText(MERGED_PROMPT_LABEL).parentElement;
    expect(coarsePointerRow?.classList.contains("min-h-11")).toBe(true);
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
    expect(screen.getByLabelText("Prompt agent when your review is requested")).not.toBeNull();
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
    hookMocks.error = "backend unavailable";
    renderPopover();

    expect(screen.getByText("backend unavailable")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(hookMocks.refreshMock).toHaveBeenCalledTimes(1);
  });
});
