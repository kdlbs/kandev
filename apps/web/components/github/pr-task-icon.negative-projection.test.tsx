import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";
import { PRTaskIcon } from "./pr-task-icon";
import type { TaskPRInfo } from "./pr-task-automation";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const APPROVAL_LABEL = "Awaiting maintainer approval";
const PR_TITLE = "Test PR";
const FIRST_REPOSITORY_PR_TITLE = "First repository PR";
const SECOND_REPOSITORY_PR_TITLE = "Second repository PR";
const STATUS_ENTRY_TEST_ID = "pr-task-status-entry";

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
    id: "pr-1",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    repository_id: "repository-1",
    owner: "o",
    repo: "r",
    pr_number: 42,
    pr_url: "",
    pr_title: PR_TITLE,
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
    last_synced_at: "2026-09-30T11:00:00Z",
    updated_at: "",
    head_sha: "head-a",
    workflow_attention: {
      state: "approval_required",
      head_sha: "head-a",
      observed_at: "2026-09-30T10:30:00Z",
      stale: true,
      runs: [],
    },
    ...overrides,
  };
}

function makePRInfo(overrides: Partial<TaskPRInfo> = {}): TaskPRInfo {
  return {
    number: 42,
    state: "open",
    count: 1,
    statusSummaryUpdatedAt: "2026-09-30T12:00:00Z",
    workflowApprovalRequired: false,
    ...overrides,
  };
}

function renderNegativeProjection(
  prs: TaskPR[],
  prInfo: TaskPRInfo,
  loadAutomationOptions = false,
) {
  const initialState: Partial<AppState> = {
    taskPRs: { byTaskId: { [TASK_ID]: prs } },
  };
  if (loadAutomationOptions) {
    initialState.workspaces = { items: [], activeId: WORKSPACE_ID };
  }
  renderWithStore(initialState, <PRTaskIcon taskId={TASK_ID} prInfo={prInfo} />);
}

function focusDisclosure() {
  const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
  const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
  fireEvent.focus(icon);
  matches.mockRestore();
  return icon;
}

function makeAutomationOptions(): TaskCIAutomationOptions {
  return {
    task_id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    auto_fix_enabled: true,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    updated_at: "2026-09-30T12:00:00Z",
    pr_states: [],
    pr_options: [
      {
        task_id: TASK_ID,
        repository_id: "repository-1",
        pr_number: 42,
        auto_fix_enabled: true,
        auto_merge_enabled: false,
        prompt_on_review_requested: false,
        prompt_on_merged: false,
        prompt_on_closed: false,
        created_at: "",
        updated_at: "",
      },
    ],
  };
}

beforeEach(() => {
  listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
  getTaskCIAutomationOptionsMock.mockReset().mockResolvedValue(undefined);
});

afterEach(() => cleanup());

// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.2/.3/.17
describe("negative projection terminal PR summaries", () => {
  it.each([
    { label: "merged", fullState: "merged", compactState: "Merged" },
    { label: "closed", fullState: "closed", compactState: "Closed" },
  ] as const)("retains the $label PR identity and status", async (scenario) => {
    renderNegativeProjection(
      [
        makePR({
          state: scenario.fullState,
          review_state: "approved",
          checks_state: "success",
        }),
      ],
      makePRInfo({ state: scenario.compactState }),
    );
    focusDisclosure();

    const entry = await screen.findByTestId(STATUS_ENTRY_TEST_ID);
    expect(entry.textContent).toContain(PR_TITLE);
    expect(entry.textContent).toContain("alice");
    expect(entry.textContent).toContain(scenario.compactState);
    expect(entry.textContent).not.toContain(APPROVAL_LABEL);
  });

  it("uses compact merged state for one matching cached PR", async () => {
    renderNegativeProjection(
      [makePR({ review_state: "approved", checks_state: "success" })],
      makePRInfo({ state: "Merged" }),
    );
    focusDisclosure();

    const entry = await screen.findByTestId(STATUS_ENTRY_TEST_ID);
    expect(entry.textContent).toContain(PR_TITLE);
    expect(entry.textContent).toContain("Merged");
    expect(entry.textContent).not.toContain("Open");
  });

  it.each([
    { compactState: "Merged", stateLabel: "Merged" },
    { compactState: "Closed", stateLabel: "Closed" },
  ])("applies the newer $stateLabel state to its matching PR among siblings", async (scenario) => {
    renderNegativeProjection(
      [
        makePR({ pr_title: "Terminal target", mergeable_state: "clean" }),
        makePR({
          id: "pr-43",
          pr_number: 43,
          pr_title: "Open sibling",
          mergeable_state: "clean",
        }),
      ],
      makePRInfo({ state: scenario.compactState, count: 2 }),
    );
    focusDisclosure();

    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    const target = entries.find((entry) => entry.textContent?.includes("Terminal target"));
    const sibling = entries.find((entry) => entry.textContent?.includes("Open sibling"));
    expect(target?.textContent).toContain(scenario.stateLabel);
    expect(target?.textContent).not.toContain("Mergeable");
    expect(target?.textContent).not.toContain("Ready to merge");
    expect(sibling?.textContent).toContain("Open");
    expect(sibling?.textContent).toContain("Mergeable");
    expect(sibling?.textContent).not.toContain(scenario.stateLabel);
  });
});

// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.6/.8
describe("negative projection open and mixed PR summaries", () => {
  it("retains open PR details when automation options are loaded", async () => {
    getTaskCIAutomationOptionsMock.mockResolvedValue(makeAutomationOptions());
    renderNegativeProjection(
      [makePR({ checks_state: "pending" })],
      makePRInfo({ autoFixEnabled: true }),
      true,
    );
    focusDisclosure();

    await screen.findByText("Auto-fix");
    const entry = await screen.findByTestId(STATUS_ENTRY_TEST_ID);
    expect(entry.textContent).toContain("PR #42");
    expect(entry.textContent).toContain(PR_TITLE);
    expect(entry.textContent).toContain("alice");
    expect(entry.textContent).toContain("In progress");
    expect(entry.textContent).not.toContain(APPROVAL_LABEL);
  });

  it("clears approval across open siblings while retaining mixed identities and count", async () => {
    renderNegativeProjection(
      [
        makePR({ id: "pr-42", pr_title: "Open PR one" }),
        makePR({
          id: "pr-43",
          pr_number: 43,
          pr_title: "Merged PR",
          state: "merged",
          workflow_attention: null,
          checks_state: "success",
        }),
        makePR({ id: "pr-44", pr_number: 44, pr_title: "Open PR two" }),
      ],
      makePRInfo({ count: 3 }),
    );
    const icon = focusDisclosure();

    expect(icon.getAttribute("data-pr-count")).toBe("3");
    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    expect(entries).toHaveLength(3);
    const content = entries.map((entry) => entry.textContent ?? "").join(" ");
    expect(content).toContain("Open PR one");
    expect(content).toContain("Merged PR");
    expect(content).toContain("Open PR two");
    expect(content).toContain("Merged");
    expect(content).not.toContain(APPROVAL_LABEL);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.6/.7
describe("negative projection compact conflict attribution", () => {
  it("matches same-number PRs by repository", async () => {
    renderNegativeProjection(
      [
        makePR({
          id: "first-42",
          repository_id: "repository-first",
          owner: "org",
          repo: "first",
          pr_title: FIRST_REPOSITORY_PR_TITLE,
          mergeable_state: "clean",
          checks_state: "success",
        }),
        makePR({
          id: "second-42",
          repository_id: "repository-second",
          owner: "org",
          repo: "second",
          pr_title: SECOND_REPOSITORY_PR_TITLE,
          mergeable_state: "clean",
          checks_state: "pending",
        }),
      ],
      makePRInfo({
        count: 2,
        hasMergeConflicts: true,
        mergeConflictRepository: "org/second",
      }),
    );
    focusDisclosure();

    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    expect(entries).toHaveLength(2);
    const first = entries.find((entry) => entry.textContent?.includes(FIRST_REPOSITORY_PR_TITLE));
    const second = entries.find((entry) => entry.textContent?.includes(SECOND_REPOSITORY_PR_TITLE));
    expect(first?.textContent).toContain("Passed");
    expect(first?.textContent).toContain("Mergeable");
    expect(first?.textContent).not.toContain("Conflicts");
    expect(second?.textContent).toContain("In progress");
    expect(second?.textContent).toContain("Conflicts");
    expect(second?.textContent).not.toContain("Mergeable");
    expect(second?.textContent).not.toContain("Ready to merge");
    expect(second?.textContent).toContain("org/second");
  });

  it("clears stale conflicts only for the repository attributed by the newer summary", async () => {
    renderNegativeProjection(
      [
        makePR({
          id: "first-42",
          repo: "first",
          pr_title: FIRST_REPOSITORY_PR_TITLE,
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "dirty",
        }),
        makePR({
          id: "second-42",
          repo: "second",
          pr_title: SECOND_REPOSITORY_PR_TITLE,
          mergeable_state: "dirty",
        }),
      ],
      makePRInfo({
        count: 2,
        hasMergeConflicts: false,
        mergeConflictPRNumber: 42,
        mergeConflictRepository: "o/first",
        workflowApprovalPRNumber: 42,
        workflowApprovalRepository: "o/second",
      }),
    );
    focusDisclosure();

    const entries = await screen.findAllByTestId(STATUS_ENTRY_TEST_ID);
    const first = entries.find((entry) => entry.textContent?.includes(FIRST_REPOSITORY_PR_TITLE));
    const second = entries.find((entry) => entry.textContent?.includes(SECOND_REPOSITORY_PR_TITLE));
    expect(first?.textContent).toContain("Approved");
    expect(first?.textContent).toContain("Passed");
    expect(first?.textContent).not.toContain("Conflicts");
    expect(second?.textContent).toContain("Conflicts");
  });
});
