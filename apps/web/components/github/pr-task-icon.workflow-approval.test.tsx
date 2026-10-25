import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import { PRTaskIcon } from "./pr-task-icon";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const ARIA_LABEL_ATTRIBUTE = "aria-label";
const APPROVAL_WARNING_TEST_ID = "pr-workflow-approval-warning";
const STATUS_ENTRY_TEST_ID = "pr-task-status-entry";
const APPROVAL_LABEL = "Awaiting maintainer approval";
const STALE_STATUS_COPY = "Last known status. GitHub could not provide a fresh workflow result.";

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
    ...overrides,
  };
}

beforeEach(() => {
  listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
  getTaskCIAutomationOptionsMock.mockReset().mockResolvedValue(undefined);
});

afterEach(() => cleanup());

const approvalAttention = {
  state: "approval_required" as const,
  head_sha: "head-a",
  observed_at: "2026-09-30T12:00:00Z",
  stale: false,
  runs: [
    {
      run_id: 1,
      run_attempt: 1,
      workflow_id: 2,
      name: "CI",
      url: "https://github.com/o/r/actions/runs/1",
      reason: "approval_required",
    },
  ],
};

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1-003.3, AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8
describe("PRTaskIcon workflow approval glyph", () => {
  it("shows the amber warning and names current-head approval", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [makePR({ head_sha: "head-a", workflow_attention: approvalAttention })],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    const warning = screen.getByTestId(APPROVAL_WARNING_TEST_ID);
    expect(warning.className).toContain("text-[#D97706]");
    expect(warning.className).toContain("dark:text-[#FBBF24]");
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain(APPROVAL_LABEL);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.5
describe("PRTaskIcon stale workflow approval disclosure", () => {
  it("keeps same-head stale approval locked and explains the last-known status by PR", async () => {
    const staleApproval = { ...approvalAttention, stale: true };
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({ pr_number: 42, head_sha: "head-a", workflow_attention: staleApproval }),
            ],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    expect(screen.getByTestId(APPROVAL_WARNING_TEST_ID)).not.toBeNull();
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain(APPROVAL_LABEL);

    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    expect(entries.length).toBeGreaterThan(0);
    expect(
      entries.every(
        (entry) =>
          entry.textContent?.includes("PR #42") && entry.textContent.includes(APPROVAL_LABEL),
      ),
    ).toBe(true);
    const staleNotes = screen.getAllByTestId("pr-task-stale-workflow-evidence");
    expect(staleNotes.length).toBeGreaterThan(0);
    expect(
      staleNotes.every(
        (staleNote) =>
          staleNote.textContent?.includes("PR #42") &&
          staleNote.textContent.includes(STALE_STATUS_COPY),
      ),
    ).toBe(true);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.2-003.3, AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.8
describe("PRTaskIcon workflow approval conflict priority", () => {
  it("keeps approval text when a conflict triangle wins the warning slot", async () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({
                head_sha: "head-a",
                workflow_attention: approvalAttention,
                has_merge_conflicts: true,
                mergeable_state: "dirty",
              }),
            ],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    expect(icon.querySelector('[data-testid="pr-merge-conflict-warning"]')).not.toBeNull();
    expect(icon.querySelector(`[data-testid="${APPROVAL_WARNING_TEST_ID}"]`)).toBeNull();
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain("Conflicts");
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain(APPROVAL_LABEL);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();
    await waitFor(() => expect(screen.getAllByText(APPROVAL_LABEL).length).toBeGreaterThan(0));
    expect(screen.getAllByText("Conflicts").length).toBeGreaterThan(0);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1-003.5
describe("PRTaskIcon compact workflow approval hydration", () => {
  it("uses compact approval while full PR records hydrate and keeps automation dots", () => {
    const prInfo = JSON.parse(
      '{"number":7,"state":"open","workflowApprovalRequired":true,"autoFixEnabled":true,"autoMergeEnabled":true}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    const warning = screen.getByTestId(APPROVAL_WARNING_TEST_ID);
    expect(warning).not.toBeNull();
    expect(icon.querySelector('[data-testid="pr-task-automation-auto-fix"]')).not.toBeNull();
    expect(icon.querySelector('[data-testid="pr-task-automation-auto-merge"]')).not.toBeNull();
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain(APPROVAL_LABEL);
  });

  it("keeps the compact approval explanation visible during hydration", async () => {
    const prInfo = JSON.parse(
      '{"number":8,"state":"open","workflowApprovalRequired":true}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();
    await waitFor(() =>
      expect(screen.getAllByTestId("pr-task-tooltip-loading").length).toBeGreaterThan(0),
    );
    expect(screen.getAllByText(APPROVAL_LABEL).length).toBeGreaterThan(0);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.3-003.5
describe("PRTaskIcon newer compact workflow projection", () => {
  it("uses the newer compact projection without mixing in an older cached PR disclosure", async () => {
    const prInfo = JSON.parse(
      '{"number":41,"state":"open","statusSummaryUpdatedAt":"2026-09-30T12:00:00Z","workflowApprovalRequired":true,"workflowApprovalPRNumber":42,"workflowApprovalRepository":"contributor/fork","workflowApprovalStale":true}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({
                pr_number: 41,
                last_synced_at: "2026-09-30T11:00:00Z",
                workflow_attention: {
                  ...approvalAttention,
                  state: "none",
                  observed_at: "2026-09-30T10:30:00Z",
                  runs: [],
                },
              }),
            ],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    expect(screen.getByTestId(APPROVAL_WARNING_TEST_ID)).not.toBeNull();
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain(APPROVAL_LABEL);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    expect(entries.length).toBeGreaterThan(0);
    expect(
      entries.every(
        (entry) =>
          entry.textContent?.includes("contributor/fork PR #42") &&
          entry.textContent.includes(APPROVAL_LABEL) &&
          !entry.textContent.includes("PR #41"),
      ),
    ).toBe(true);
    const staleNotes = screen.getAllByTestId("pr-task-stale-workflow-evidence");
    expect(staleNotes.length).toBeGreaterThan(0);
    expect(
      staleNotes.every(
        (staleNote) =>
          staleNote.textContent?.includes("contributor/fork PR #42") &&
          staleNote.textContent.includes(STALE_STATUS_COPY),
      ),
    ).toBe(true);
  });

  it("uses a newer explicit negative compact projection to clear older stale approval", async () => {
    const prInfo = JSON.parse(
      '{"number":42,"state":"open","statusSummaryUpdatedAt":"2026-09-30T12:00:00Z","workflowApprovalRequired":false}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({
                pr_number: 42,
                repository_id: "repository-1",
                last_synced_at: "2026-09-30T11:00:00Z",
                head_sha: "head-a",
                review_state: "approved",
                checks_state: "success",
                workflow_attention: {
                  ...approvalAttention,
                  observed_at: "2026-09-30T10:30:00Z",
                  stale: true,
                },
              }),
            ],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    expect(screen.queryByTestId(APPROVAL_WARNING_TEST_ID)).toBeNull();
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();
    await waitFor(() => expect(screen.queryByText(APPROVAL_LABEL)).toBeNull());
    expect(screen.queryByTestId("pr-task-stale-workflow-evidence")).toBeNull();

    const entry = await screen.findByTestId(STATUS_ENTRY_TEST_ID);
    expect(entry.textContent).toContain("PR #42");
    expect(entry.textContent).toContain("Test PR");
    expect(entry.textContent).toContain("alice");
    expect(entry.textContent).toContain("Approved");
    expect(entry.textContent).toContain("Passed");
  });
});

describe("PRTaskIcon current-head workflow approval evidence", () => {
  it("keeps current full evidence authoritative when it is newer than the compact projection", () => {
    const prInfo = JSON.parse(
      '{"number":9,"state":"open","statusSummaryUpdatedAt":"2026-09-30T12:00:00Z","workflowApprovalRequired":true,"workflowApprovalPRNumber":9}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    const currentNone = {
      state: "none" as const,
      head_sha: "head-b",
      observed_at: "2026-09-30T13:00:00Z",
      stale: false,
      runs: [],
    };
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({
                head_sha: "head-b",
                last_synced_at: "2026-09-30T13:00:00Z",
                workflow_attention: currentNone,
              }),
            ],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    expect(screen.queryByTestId(APPROVAL_WARNING_TEST_ID)).toBeNull();
  });

  it("keeps compact approval and conflict reasons attributed to their respective PRs", async () => {
    const prInfo = JSON.parse(
      '{"number":41,"state":"open","statusSummaryUpdatedAt":"2026-09-30T12:00:00Z","workflowApprovalRequired":true,"workflowApprovalPRNumber":42,"workflowApprovalRepository":"contributor/fork","hasMergeConflicts":true,"mergeConflictPRNumber":43,"mergeConflictRepository":"org/repo"}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [makePR({ pr_number: 41, last_synced_at: "2026-09-30T11:00:00Z" })],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    expect(icon.querySelector('[data-testid="pr-merge-conflict-warning"]')).not.toBeNull();
    expect(icon.querySelector(`[data-testid="${APPROVAL_WARNING_TEST_ID}"]`)).toBeNull();
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain("Conflicts");
    expect(icon.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain(APPROVAL_LABEL);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    expect(
      entries.some(
        (entry) =>
          entry.textContent?.includes("contributor/fork PR #42") &&
          entry.textContent.includes(APPROVAL_LABEL),
      ),
    ).toBe(true);
    expect(
      entries.some(
        (entry) =>
          entry.textContent?.includes("org/repo PR #43") && entry.textContent.includes("Conflicts"),
      ),
    ).toBe(true);
  });
});

describe("PRTaskIcon hydrated workflow approval evidence", () => {
  it("uses hydrated negative evidence instead of a positive compact flag", () => {
    const prInfo = JSON.parse(
      '{"number":9,"state":"open","workflowApprovalRequired":true}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    const currentNone = {
      state: "none" as const,
      head_sha: "head-b",
      observed_at: "2026-09-30T12:00:00Z",
      stale: false,
      runs: [],
    };
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [makePR({ head_sha: "head-b", workflow_attention: currentNone })],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    expect(screen.queryByTestId(APPROVAL_WARNING_TEST_ID)).toBeNull();
    expect(screen.queryByText(STALE_STATUS_COPY)).toBeNull();
  });

  it("clears stale approval when its evidence belongs to a different head", () => {
    const prInfo = JSON.parse(
      '{"number":10,"state":"open","workflowApprovalRequired":true}',
    ) as NonNullable<Parameters<typeof PRTaskIcon>[0]["prInfo"]>;
    const stalePreviousHead = { ...approvalAttention, stale: true };
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [makePR({ head_sha: "head-b", workflow_attention: stalePreviousHead })],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />,
    );

    expect(screen.queryByTestId(APPROVAL_WARNING_TEST_ID)).toBeNull();
    expect(screen.queryByText(STALE_STATUS_COPY)).toBeNull();
  });
});
