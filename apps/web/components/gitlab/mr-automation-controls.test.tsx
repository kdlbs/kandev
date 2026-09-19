import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import type {
  TaskMR,
  TaskMRAutomationOptions,
  TaskMRAutomationOptionsForMR,
} from "@/lib/types/gitlab";

const hookMocks = vi.hoisted(() => ({
  error: null as string | null,
  options: null as TaskMRAutomationOptions | null,
  loading: false,
  updateMock: vi.fn(),
  refreshMock: vi.fn(),
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

vi.mock("@/hooks/domains/gitlab/use-task-mr-automation", () => ({
  useTaskMRAutomationOptions: () => ({
    options: hookMocks.options,
    loading: hookMocks.loading,
    saving: false,
    error: hookMocks.error,
    refresh: hookMocks.refreshMock,
    update: hookMocks.updateMock,
    resetPrompt: hookMocks.resetPromptMock,
  }),
}));

import { MRAutomationControls } from "./mr-automation-controls";

const MERGED_LABEL = "MR merged";
const CLOSED_LABEL = "MR closed without merging";
const REVIEW_REQUESTED_LABEL = "Your review is requested";
const FOLLOW_UP_TRIGGER = "mr-review-follow-up-trigger";

function makeMROptions(
  overrides: Partial<TaskMRAutomationOptionsForMR> = {},
): TaskMRAutomationOptionsForMR {
  return {
    task_id: "task-1",
    repository_id: "",
    project_path: "group/project",
    mr_iid: 7,
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeOptions(overrides: Partial<TaskMRAutomationOptions> = {}): TaskMRAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_max_rounds: 10,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    review_reviewer_username: "",
    updated_at: "2026-06-18T10:00:00Z",
    mr_states: [],
    mr_options: [makeMROptions()],
    ...overrides,
  };
}

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "assoc-1",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 7,
    mr_url: "https://gitlab.com/group/project/-/merge_requests/7",
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

function renderControls() {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MRAutomationControls taskId="task-1" mr={makeMR()} />
      </ToastProvider>
    </TooltipProvider>,
  );
}

function resetHookMocks() {
  hookMocks.error = null;
  hookMocks.options = makeOptions();
  hookMocks.loading = false;
  hookMocks.updateMock.mockReset();
  hookMocks.updateMock.mockResolvedValue(makeOptions());
  hookMocks.refreshMock.mockReset();
  hookMocks.refreshMock.mockResolvedValue(makeOptions());
  hookMocks.resetPromptMock.mockReset();
  hookMocks.resetPromptMock.mockResolvedValue(makeOptions());
  responsiveMock.isFinePointer = true;
  responsiveMock.isMobile = false;
}

describe("MRAutomationControls", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("keeps the review follow-up group collapsed until opened", () => {
    renderControls();
    const trigger = screen.getByTestId(FOLLOW_UP_TRIGGER);

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText(REVIEW_REQUESTED_LABEL)).toBeNull();
    expect(screen.queryByLabelText(MERGED_LABEL)).toBeNull();
    expect(screen.queryByLabelText(CLOSED_LABEL)).toBeNull();

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText(REVIEW_REQUESTED_LABEL)).not.toBeNull();
    expect(screen.getByLabelText(MERGED_LABEL)).not.toBeNull();
    expect(screen.getByLabelText(CLOSED_LABEL)).not.toBeNull();
  });

  it("auto-expands when a switch is already on", () => {
    hookMocks.options = makeOptions({ mr_options: [makeMROptions({ prompt_on_merged: true })] });
    renderControls();
    expect(screen.getByTestId(FOLLOW_UP_TRIGGER).getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByLabelText(MERGED_LABEL)).not.toBeNull();
  });

  it("shows persistent plain-language copy explaining the switch group, not just on hover/tap", () => {
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));

    // Visible as soon as the group is open — no help-icon interaction
    // required (apps/web/AGENTS.md's self-documenting-settings convention).
    expect(
      screen.getByText(/Wakes this task's agent when your review is requested/),
    ).not.toBeNull();
  });

  it("wires aria-describedby to sr-only descriptions for each row", () => {
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));

    expect(screen.getByLabelText(REVIEW_REQUESTED_LABEL).getAttribute("aria-describedby")).toBe(
      "task-mr-review-requested-prompt-task-1-assoc-1-description",
    );
    expect(screen.getByLabelText(MERGED_LABEL).getAttribute("aria-describedby")).toBe(
      "task-mr-terminal-help-task-1-assoc-1",
    );
    expect(screen.getByLabelText(CLOSED_LABEL).getAttribute("aria-describedby")).toBe(
      "task-mr-terminal-help-task-1-assoc-1",
    );
    expect(
      screen.getByText(
        "Wake the agent when the workspace's connected GitLab account is added as a reviewer. Being re-added after removal counts as a new request; staying assigned across MR updates does not.",
      ),
    ).not.toBeNull();
    expect(
      screen.getByText("Wake the agent when review work ends. Choose either or both outcomes."),
    ).not.toBeNull();
  });

  it("toggles each switch and calls update with the right patch", () => {
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));

    fireEvent.click(screen.getByLabelText(REVIEW_REQUESTED_LABEL));
    fireEvent.click(screen.getByLabelText(MERGED_LABEL));
    fireEvent.click(screen.getByLabelText(CLOSED_LABEL));

    const mrIdentity = { repository_id: "", project_path: "group/project", mr_iid: 7 };
    expect(hookMocks.updateMock).toHaveBeenCalledWith({
      ...mrIdentity,
      prompt_on_review_requested: true,
    });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ ...mrIdentity, prompt_on_merged: true });
    expect(hookMocks.updateMock).toHaveBeenCalledWith({ ...mrIdentity, prompt_on_closed: true });
  });
});

describe("MRAutomationControls error and loading states", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("surfaces an error from the hook", () => {
    hookMocks.error = "network down";
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));
    expect(screen.getByRole("alert").textContent).toContain("network down");
  });

  it("shows a retry banner outside the collapsed group when the initial load fails", () => {
    hookMocks.options = null;
    hookMocks.error = "initial load failed";
    renderControls();

    // Visible without opening the collapsible group — the group never
    // auto-expands with no loaded options, so a nested error would be
    // invisible in the default collapsed state.
    expect(screen.getByRole("alert").textContent).toContain("initial load failed");
    fireEvent.click(screen.getByTestId("mr-automation-retry"));
    expect(hookMocks.refreshMock).toHaveBeenCalledTimes(1);
  });

  it("does not duplicate the error banner once options have loaded (save/update error)", () => {
    hookMocks.options = makeOptions();
    hookMocks.error = "save failed";
    renderControls();

    expect(screen.queryByTestId("mr-automation-retry")).toBeNull();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });

  it("disables the switches until options have loaded", () => {
    hookMocks.options = null;
    hookMocks.loading = true;
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));

    expect((screen.getByLabelText(REVIEW_REQUESTED_LABEL) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect((screen.getByLabelText(MERGED_LABEL) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByLabelText(CLOSED_LABEL) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("MRAutomationControls help affordance", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("shows the help affordance via keyboard focus (AC29)", () => {
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));

    const helpButton = screen.getByTestId("mr-review-requested-help");
    expect(helpButton).not.toBeNull();
    fireEvent.focus(helpButton);
    // happy-dom opens Radix Tooltip on focus (unlike jsdom — see
    // apps/web/CLAUDE.md); assert the tooltip's role="tooltip" surface
    // rendered with the help copy.
    expect(screen.getByRole("tooltip").textContent).toContain(
      "Wake the agent when the workspace's connected GitLab account is added as a reviewer",
    );
  });

  it("renders a coarse-pointer tap popover instead of a hover tooltip", () => {
    responsiveMock.isFinePointer = false;
    renderControls();
    fireEvent.click(screen.getByTestId(FOLLOW_UP_TRIGGER));

    const helpButton = screen.getByTestId("mr-review-requested-help");
    fireEvent.click(helpButton);
    // The sr-only description and the popover both carry the same copy;
    // assert the popover's role="dialog" surface rendered at all.
    expect(helpButton.getAttribute("aria-haspopup")).toBe("dialog");
    expect(
      screen.getAllByText(
        "Wake the agent when the workspace's connected GitLab account is added as a reviewer. Being re-added after removal counts as a new request; staying assigned across MR updates does not.",
      ).length,
    ).toBeGreaterThan(1);
  });

  it("keeps the trigger compact on desktop and touch-sized on mobile", () => {
    renderControls();
    const desktopTrigger = screen.getByTestId(FOLLOW_UP_TRIGGER);
    expect(desktopTrigger.classList.contains("min-h-7")).toBe(true);

    cleanup();
    responsiveMock.isMobile = true;
    renderControls();
    expect(screen.getByTestId(FOLLOW_UP_TRIGGER).classList.contains("min-h-11")).toBe(true);
  });
});
