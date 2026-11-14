import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import { PRTaskIcon } from "./pr-task-icon";
import type { TaskPRInfo } from "./pr-task-automation";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const ICON_TEST_ID = `pr-task-icon-${TASK_ID}`;
const FAILED_BUBBLE = "pr-ci-bubble-failed";
const IN_PROGRESS_BUBBLE = "pr-ci-bubble-in-progress";
const ATTENTION_BUBBLE = "pr-ci-bubble-attention";
const APPROVAL_BUBBLE = "pr-ci-bubble-approval-required";
const CONFLICT = "pr-merge-conflict-warning";
const OBSERVED_AT = "2026-11-10T12:00:00Z";

vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: listTaskPRsMock,
  getTaskCIAutomationOptions: getTaskCIAutomationOptionsMock,
}));

function renderWithStore(initialState: Partial<AppState> | undefined, ui: ReactNode) {
  return render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>{ui}</TooltipProvider>
    </StateProvider>,
  );
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    owner: "acme",
    repo: "widget",
    pr_number: 42,
    pr_url: "https://github.com/acme/widget/pull/42",
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
    head_sha: "head-a",
    ...overrides,
  };
}

function renderPR(pr: TaskPR, prInfo?: TaskPRInfo) {
  renderWithStore(
    { taskPRs: { byTaskId: { [TASK_ID]: [pr] } } },
    <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
  );
  return screen.getByTestId(ICON_TEST_ID);
}

function attention(state: "approval_required" | "action_required", stale = false) {
  return {
    state,
    head_sha: "head-a",
    observed_at: OBSERVED_AT,
    stale,
    runs: [],
  };
}

beforeEach(() => {
  listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
  getTaskCIAutomationOptionsMock.mockReset().mockResolvedValue(undefined);
});

afterEach(() => cleanup());

describe("PRTaskIcon CI bubble", () => {
  it("keeps a draft glyph muted while showing a failure bubble", () => {
    const icon = renderPR(makePR({ mergeable_state: "draft", checks_state: "failure" }));
    expect(icon.className).toContain("text-muted-foreground");
    expect(screen.getByTestId(FAILED_BUBBLE)).not.toBeNull();
  });

  it("shows no bubble for successful CI", () => {
    renderPR(makePR({ checks_state: "success" }));
    expect(screen.queryByTestId(FAILED_BUBBLE)).toBeNull();
    expect(screen.queryByTestId(IN_PROGRESS_BUBBLE)).toBeNull();
    expect(screen.queryByTestId(ATTENTION_BUBBLE)).toBeNull();
    expect(screen.queryByTestId(APPROVAL_BUBBLE)).toBeNull();
  });

  it("shows no bubble for all-skipped metadata", () => {
    renderPR(
      makePR({
        checks_state: "",
        check_summary: { head_sha: "head-a", observed: 3, skipped: 3 },
      }),
    );
    expect(screen.queryByTestId(FAILED_BUBBLE)).toBeNull();
    expect(screen.queryByTestId(IN_PROGRESS_BUBBLE)).toBeNull();
  });

  it("shows a progress ring for running CI", () => {
    renderPR(makePR({ checks_state: "pending" }));
    const bubble = screen.getByTestId(IN_PROGRESS_BUBBLE);
    expect(bubble.querySelector(".motion-safe\\:animate-spin")).not.toBeNull();
  });

  it("distinguishes approval from other workflow attention", () => {
    renderPR(makePR({ workflow_attention: attention("approval_required") }));
    expect(screen.getByTestId(APPROVAL_BUBBLE)).not.toBeNull();
    cleanup();
    renderPR(makePR({ workflow_attention: attention("action_required") }));
    expect(screen.getByTestId(ATTENTION_BUBBLE)).not.toBeNull();
  });

  it("prioritizes the conflict triangle over the CI bubble", () => {
    renderPR(
      makePR({
        mergeable_state: "dirty",
        has_merge_conflicts: true,
        workflow_attention: attention("approval_required"),
      }),
    );
    expect(screen.getByTestId(CONFLICT)).not.toBeNull();
    expect(screen.queryByTestId(APPROVAL_BUBBLE)).toBeNull();
  });

  it("keeps CI details and automation dots when conflict owns the badge", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({
                checks_state: "failure",
                has_merge_conflicts: true,
                mergeable_state: "dirty",
              }),
            ],
          },
        },
      },
      <PRTaskIcon
        taskId={TASK_ID}
        prInfo={{ number: 42, state: "open", autoFixEnabled: true, autoMergeEnabled: true }}
      />,
    );
    expect(screen.queryByTestId(FAILED_BUBBLE)).toBeNull();
    expect(screen.getByTestId(CONFLICT)).not.toBeNull();
    expect(screen.getByTestId("pr-task-automation-auto-fix")).not.toBeNull();
    expect(screen.getByTestId("pr-task-automation-auto-merge")).not.toBeNull();
    const icon = screen.getByTestId(ICON_TEST_ID);
    expect(icon.getAttribute("aria-label")).toContain("Checks failed");
    expect(icon.getAttribute("aria-label")).toContain("Conflicts");
  });
});

describe("PRTaskIcon compact CI authority", () => {
  it("renders a compact failure bubble before full records hydrate", () => {
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <PRTaskIcon
        taskId={TASK_ID}
        prInfo={{ number: 42, state: "open", ciState: "failed", ciPRNumber: 42 }}
      />,
    );
    expect(screen.getByTestId(FAILED_BUBBLE)).not.toBeNull();
  });

  it("lets a newer compact none clear an older full failure", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({ checks_state: "failure", last_synced_at: "2026-11-10T11:00:00Z" }),
            ],
          },
        },
      },
      <PRTaskIcon
        taskId={TASK_ID}
        prInfo={{
          number: 42,
          state: "open",
          ciState: "none",
          statusSummaryUpdatedAt: OBSERVED_AT,
        }}
      />,
    );
    expect(screen.queryByTestId(FAILED_BUBBLE)).toBeNull();
  });
});

describe("PRTaskIcon compact CI disclosure freshness", () => {
  it("keeps full failure authority when the compact timestamp is older", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [makePR({ checks_state: "failure", last_synced_at: OBSERVED_AT })],
          },
        },
      },
      <PRTaskIcon
        taskId={TASK_ID}
        prInfo={{
          number: 42,
          state: "open",
          ciState: "none",
          statusSummaryUpdatedAt: "2026-11-10T11:00:00Z",
        }}
      />,
    );
    expect(screen.getByTestId(FAILED_BUBBLE)).not.toBeNull();
  });

  it("replaces a stale full failure with a newer compact in-progress disclosure", async () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({ checks_state: "failure", last_synced_at: "2026-11-10T11:00:00Z" }),
            ],
          },
        },
      },
      <PRTaskIcon
        taskId={TASK_ID}
        prInfo={{
          number: 42,
          state: "open",
          ciState: "in_progress",
          statusSummaryUpdatedAt: OBSERVED_AT,
        }}
      />,
    );
    const icon = screen.getByTestId(ICON_TEST_ID);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    const compactRow = await screen.findByTestId("pr-task-status-ci-ci-state");
    expect(compactRow.textContent).toContain("In progress");
    expect(screen.queryByTestId("pr-task-status-ci")).toBeNull();
  });

  it("ignores a compact CI field with a malformed timestamp", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [makePR({ checks_state: "failure", last_synced_at: OBSERVED_AT })],
          },
        },
      },
      <PRTaskIcon
        taskId={TASK_ID}
        prInfo={{
          number: 42,
          state: "open",
          ciState: "none",
          statusSummaryUpdatedAt: "0",
        }}
      />,
    );
    expect(screen.getByTestId(FAILED_BUBBLE)).not.toBeNull();
  });
});
