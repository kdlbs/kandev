import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import type {
  TaskMR,
  TaskMRAutomationOptions,
  TaskMRAutomationOptionsForMR,
  TaskMRLifecycleState,
} from "@/lib/types/gitlab";

const hookMocks = vi.hoisted(() => ({
  error: null as string | null,
  options: null as TaskMRAutomationOptions | null,
  loading: false,
  saving: false,
  updateMock: vi.fn(),
  refreshMock: vi.fn(),
  resetPromptMock: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true, isMobile: false }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr-automation", () => ({
  useTaskMRAutomationOptions: () => ({
    options: hookMocks.options,
    loading: hookMocks.loading,
    saving: hookMocks.saving,
    error: hookMocks.error,
    refresh: hookMocks.refreshMock,
    update: hookMocks.updateMock,
    resetPrompt: hookMocks.resetPromptMock,
  }),
}));

import { MRAutomationControls } from "./mr-automation-controls";

const AUTO_FIX_LABEL = "Auto-fix CI and address comments";
const AUTO_MERGE_LABEL = "Auto-merge when ready";
const EDIT_PROMPT_LABEL = "Edit auto-fix prompt for this task";
const PROMPT_TEXTAREA_LABEL = "Task auto-fix prompt";
const PROJECT_PATH = "group/project";

function makeMROptions(
  overrides: Partial<TaskMRAutomationOptionsForMR> = {},
): TaskMRAutomationOptionsForMR {
  return {
    task_id: "task-1",
    repository_id: "",
    project_path: PROJECT_PATH,
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
    effective_auto_fix_prompt: "default prompt",
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
    project_path: PROJECT_PATH,
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

function makeState(overrides: Partial<TaskMRLifecycleState> = {}): TaskMRLifecycleState {
  return {
    task_id: "task-1",
    repository_id: "",
    project_path: PROJECT_PATH,
    mr_iid: 7,
    review_request_initialized: false,
    last_review_requested: false,
    last_observed_state: "",
    last_lifecycle_event: "",
    last_fix_signature: "",
    last_fix_checkpoint_json: "",
    auto_fix_round_count: 0,
    last_merge_signature: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function renderControls(mr: TaskMR = makeMR()) {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <MRAutomationControls taskId="task-1" mr={mr} />
      </ToastProvider>
    </TooltipProvider>,
  );
}

function resetHookMocks() {
  hookMocks.error = null;
  hookMocks.options = makeOptions();
  hookMocks.loading = false;
  hookMocks.saving = false;
  hookMocks.updateMock.mockReset();
  hookMocks.updateMock.mockResolvedValue(makeOptions());
  hookMocks.refreshMock.mockReset();
  hookMocks.refreshMock.mockResolvedValue(makeOptions());
  hookMocks.resetPromptMock.mockReset();
  hookMocks.resetPromptMock.mockResolvedValue(makeOptions());
}

describe("MRAutomationControls — Automation section (AC1)", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the Automation header with exactly two automation switches above Review follow-up", () => {
    renderControls();
    expect(screen.getByText("Automation")).toBeTruthy();
    expect(screen.getByLabelText(AUTO_FIX_LABEL)).toBeTruthy();
    expect(screen.getByLabelText(AUTO_MERGE_LABEL)).toBeTruthy();
    expect(screen.getByTestId("mr-review-follow-up-trigger")).toBeTruthy();
  });

  it("toggling auto-fix on patches auto_fix_enabled", () => {
    renderControls();
    fireEvent.click(screen.getByLabelText(AUTO_FIX_LABEL));
    expect(hookMocks.updateMock).toHaveBeenCalledWith({
      repository_id: "",
      project_path: PROJECT_PATH,
      mr_iid: 7,
      auto_fix_enabled: true,
    });
  });

  it("toggling auto-merge on patches auto_merge_enabled", () => {
    renderControls();
    fireEvent.click(screen.getByLabelText(AUTO_MERGE_LABEL));
    expect(hookMocks.updateMock).toHaveBeenCalledWith({
      repository_id: "",
      project_path: PROJECT_PATH,
      mr_iid: 7,
      auto_merge_enabled: true,
    });
  });

  it("shows the round-help button only when auto-fix is enabled and a single MR is known", () => {
    hookMocks.options = makeOptions({ mr_options: [makeMROptions({ auto_fix_enabled: false })] });
    renderControls();
    expect(screen.queryByTestId("mr-auto-fix-round-help")).toBeNull();

    cleanup();
    hookMocks.options = makeOptions({
      mr_options: [makeMROptions({ auto_fix_enabled: true })],
      mr_states: [makeState({ auto_fix_round_count: 3 })],
    });
    renderControls();
    expect(screen.getByTestId("mr-auto-fix-round-help")).toBeTruthy();
  });

  it("shows the auto-merge readiness help button only when auto-merge is enabled", () => {
    hookMocks.options = makeOptions({
      mr_options: [makeMROptions({ auto_merge_enabled: false })],
    });
    renderControls();
    expect(screen.queryByTestId("mr-auto-merge-help")).toBeNull();

    cleanup();
    hookMocks.options = makeOptions({ mr_options: [makeMROptions({ auto_merge_enabled: true })] });
    renderControls();
    expect(screen.getByTestId("mr-auto-merge-help")).toBeTruthy();
  });

  it("renders switches off when options have no entry for this MR", () => {
    hookMocks.options = makeOptions({
      mr_options: [
        makeMROptions({
          mr_iid: 99,
          auto_fix_enabled: true,
          auto_merge_enabled: true,
        }),
      ],
    });
    renderControls();
    expect(screen.getByLabelText(AUTO_FIX_LABEL).getAttribute("data-state")).toBe("unchecked");
    expect(screen.getByLabelText(AUTO_MERGE_LABEL).getAttribute("data-state")).toBe("unchecked");
    expect(screen.queryByTestId("mr-auto-fix-round-help")).toBeNull();
    expect(screen.queryByTestId("mr-auto-merge-help")).toBeNull();
  });

  it("disables both automation switches while loading", () => {
    hookMocks.loading = true;
    hookMocks.options = null;
    renderControls();
    expect(screen.getByLabelText(AUTO_FIX_LABEL)).toHaveProperty("disabled", true);
    expect(screen.getByLabelText(AUTO_MERGE_LABEL)).toHaveProperty("disabled", true);
  });
});

describe("MRAutomationControls — auto-fix prompt editor", () => {
  beforeEach(() => {
    resetHookMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("opens the auto-fix prompt dialog with the effective prompt pre-filled", () => {
    hookMocks.options = makeOptions({ effective_auto_fix_prompt: "custom prompt text" });
    renderControls();
    fireEvent.click(screen.getByLabelText(EDIT_PROMPT_LABEL));
    const textarea = screen.getByLabelText(PROMPT_TEXTAREA_LABEL) as HTMLTextAreaElement;
    expect(textarea.value).toBe("custom prompt text");
  });

  it("opens on the effective prompt when the stored override is whitespace-only", () => {
    // A whitespace-only override resolves to the default server-side
    // (using_default_prompt: true), so opening the editor on that raw
    // whitespace would show a blank textarea with Save disabled.
    hookMocks.options = makeOptions({
      auto_fix_prompt_override: "   ",
      effective_auto_fix_prompt: "the default prompt",
      using_default_prompt: true,
    });
    renderControls();
    fireEvent.click(screen.getByLabelText(EDIT_PROMPT_LABEL));
    const textarea = screen.getByLabelText(PROMPT_TEXTAREA_LABEL) as HTMLTextAreaElement;
    expect(textarea.value).toBe("the default prompt");
  });

  it("opens on a real override when one is set", () => {
    hookMocks.options = makeOptions({
      auto_fix_prompt_override: "my override",
      effective_auto_fix_prompt: "my override",
    });
    renderControls();
    fireEvent.click(screen.getByLabelText(EDIT_PROMPT_LABEL));
    const textarea = screen.getByLabelText(PROMPT_TEXTAREA_LABEL) as HTMLTextAreaElement;
    expect(textarea.value).toBe("my override");
  });

  it("saves a new prompt override", () => {
    renderControls();
    fireEvent.click(screen.getByLabelText(EDIT_PROMPT_LABEL));
    const textarea = screen.getByLabelText(PROMPT_TEXTAREA_LABEL) as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "a new override" } });
    fireEvent.click(screen.getByText("Save prompt"));
    expect(hookMocks.updateMock).toHaveBeenCalledWith({
      auto_fix_prompt_override: "a new override",
    });
  });

  it("restores the default prompt via Use default", () => {
    renderControls();
    fireEvent.click(screen.getByLabelText(EDIT_PROMPT_LABEL));
    fireEvent.click(screen.getByText("Use default"));
    expect(hookMocks.resetPromptMock).toHaveBeenCalled();
  });
});
