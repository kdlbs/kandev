import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { TaskContributionIcons } from "@/components/task/task-contribution-icons";
import type { AppState } from "@/lib/state/store";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

const apiMocks = vi.hoisted(() => ({
  listTaskPRs: vi.fn(),
  getTaskCIAutomationOptions: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: apiMocks.listTaskPRs,
  getTaskCIAutomationOptions: apiMocks.getTaskCIAutomationOptions,
}));

const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";

function renderWithStore(initialState: Partial<AppState>, children: React.ReactNode) {
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>{children}</TooltipProvider>
    </StateProvider>,
  );
}

function makePR(): TaskPR {
  return {
    id: "pr-1",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    repository_id: "repo-1",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
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

function makeAutomationOptions(): TaskCIAutomationOptions {
  return {
    task_id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    updated_at: "2026-08-01T00:00:00Z",
    pr_states: [],
    pr_options: [
      {
        task_id: TASK_ID,
        repository_id: "repo-1",
        pr_number: 1,
        auto_fix_enabled: true,
        auto_merge_enabled: true,
        prompt_on_review_requested: false,
        prompt_on_merged: false,
        prompt_on_closed: false,
        created_at: "",
        updated_at: "",
      },
    ],
  };
}

afterEach(() => cleanup());

describe("PRTaskIcon automation disclosure", () => {
  it("shows pending automation settings before hiding a disabled section", async () => {
    let resolveOptions!: (value: TaskCIAutomationOptions) => void;
    const response = new Promise<TaskCIAutomationOptions>((resolve) => {
      resolveOptions = resolve;
    });
    apiMocks.getTaskCIAutomationOptions.mockReset().mockReturnValue(response);
    renderWithStore(
      {
        workspaces: { items: [], activeId: WORKSPACE_ID },
        taskPRs: { byTaskId: { [TASK_ID]: [makePR()] } },
      },
      <TaskContributionIcons taskId={TASK_ID} prInfo={{ number: 1, state: "open" }} />,
    );

    fireEvent.pointerEnter(screen.getByTestId(`pr-task-icon-${TASK_ID}`), {
      pointerType: "mouse",
    });
    await waitFor(() => expect(apiMocks.getTaskCIAutomationOptions).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId("pr-task-automation-details").textContent).toContain(
      "Loading pull request details",
    );

    await act(async () => {
      resolveOptions({ ...makeAutomationOptions(), pr_options: [] });
      await response;
    });
    await waitFor(() => expect(screen.queryByTestId("pr-task-automation-details")).toBeNull());
  });

  // @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.29
  it("loads per-PR automation settings when full PR records are already cached", async () => {
    apiMocks.listTaskPRs.mockReset();
    apiMocks.getTaskCIAutomationOptions.mockReset().mockResolvedValue(makeAutomationOptions());
    renderWithStore(
      {
        workspaces: { items: [], activeId: WORKSPACE_ID },
        taskPRs: { byTaskId: { [TASK_ID]: [makePR()] } },
      },
      <TaskContributionIcons taskId={TASK_ID} prInfo={{ number: 1, state: "open" }} />,
    );

    fireEvent.pointerEnter(screen.getByTestId(`pr-task-icon-${TASK_ID}`), {
      pointerType: "mouse",
    });

    await waitFor(() => expect(apiMocks.getTaskCIAutomationOptions).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByTestId("pr-task-automation-details").textContent).toContain("Auto-fix"),
    );
    expect(apiMocks.listTaskPRs).not.toHaveBeenCalled();
  });
});
