import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { TaskContributionIcons } from "@/components/task/task-contribution-icons";
import { PRTaskIcon } from "./pr-task-icon";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import type { TaskCIAutomationOptions } from "@/lib/types/github";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";

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

function makeAutomationOptions(
  overrides: Partial<TaskCIAutomationOptions> = {},
): TaskCIAutomationOptions {
  return {
    task_id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    auto_fix_enabled: true,
    auto_merge_enabled: true,
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
    ...overrides,
  };
}

beforeEach(() => {
  listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
  getTaskCIAutomationOptionsMock.mockReset().mockResolvedValue(undefined);
});

afterEach(() => cleanup());

describe("PRTaskIcon corrupted store entry", () => {
  // Regression: an upstream payload (partial hydration, WS reorder, etc.) once
  // landed in taskPRs.byTaskId[TASK_ID] as a non-array truthy value. The
  // length-based guards then fell through into MultiPRIcon, where for-of
  // threw `prs is not iterable`. PRTaskIcon must bail rather than crash.
  it("renders nothing when byTaskId[taskId] is a non-array object", () => {
    const { container } = renderWithStore(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      { taskPRs: { byTaskId: { [TASK_ID]: {} as any } } } as Partial<AppState>,
      <PRTaskIcon taskId={TASK_ID} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when byTaskId[taskId] is undefined", () => {
    const { container } = renderWithStore(undefined, <PRTaskIcon taskId="missing" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders an icon when byTaskId[taskId] is a valid array of one PR", () => {
    const { container } = renderWithStore(
      { taskPRs: { byTaskId: { [TASK_ID]: [makePR()] } } },
      <PRTaskIcon taskId={TASK_ID} />,
    );
    expect(container.querySelector(`[data-testid="pr-task-icon-${TASK_ID}"]`)).not.toBeNull();
  });

  it("renders the multi-PR icon when byTaskId[taskId] has multiple PRs", () => {
    const { container } = renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            [TASK_ID]: [
              makePR({ id: "a", repository_id: "repo-a", pr_number: 1 }),
              makePR({ id: "b", repository_id: "repo-b", pr_number: 2 }),
            ],
          },
        },
      },
      <PRTaskIcon taskId={TASK_ID} />,
    );
    const icon = container.querySelector(`[data-testid="pr-task-icon-${TASK_ID}"]`);
    expect(icon).not.toBeNull();
    expect(icon?.getAttribute("data-pr-count")).toBe("2");
    expect(icon?.getAttribute("data-pr-ready-to-merge")).toBe("false");
  });

  it("opens a loading disclosure for a compact PR projection", () => {
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons
        taskId={TASK_ID}
        prInfo={{ number: 7, state: "open", aggregateState: "pending" }}
      />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    expect(icon.getAttribute("role")).toBe("img");
    fireEvent.pointerEnter(icon, { pointerType: "mouse" });

    expect(screen.getAllByTestId("pr-task-tooltip-loading").length).toBeGreaterThan(0);
  });

  it("opens a loading disclosure when a compact PR projection receives keyboard focus", () => {
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons
        taskId={TASK_ID}
        prInfo={{ number: 7, state: "open", aggregateState: "pending" }}
      />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    expect(screen.getAllByTestId("pr-task-tooltip-loading").length).toBeGreaterThan(0);
  });

  it("keeps keyboard focus and the open tooltip when hydration completes", async () => {
    let resolveResponse!: (value: { task_prs: Record<string, TaskPR[]> }) => void;
    const response = new Promise<{ task_prs: Record<string, TaskPR[]> }>((resolve) => {
      resolveResponse = resolve;
    });
    listTaskPRsMock.mockReturnValue(response);
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons
        taskId={TASK_ID}
        prInfo={{ number: 7, state: "open", aggregateState: "pending" }}
      />,
    );

    const icon = screen.getByTestId(`pr-task-icon-${TASK_ID}`);
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    icon.focus();
    matches.mockRestore();

    await waitFor(() =>
      expect(screen.getAllByTestId("pr-task-tooltip-loading").length).toBeGreaterThan(0),
    );
    await act(async () => {
      resolveResponse({ task_prs: { [TASK_ID]: [makePR()] } });
      await response;
    });

    await waitFor(() =>
      expect(screen.getAllByTestId("pr-task-status-summary").length).toBeGreaterThan(0),
    );
    expect(document.activeElement).toBe(screen.getByTestId(`pr-task-icon-${TASK_ID}`));
  });
});

describe("PRTaskIcon automation indicators", () => {
  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
  it("renders independent automation dots from the bounded row projection", () => {
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons
        taskId={TASK_ID}
        prInfo={{ number: 7, state: "open", autoFixEnabled: true, autoMergeEnabled: true }}
      />,
    );

    expect(screen.getByTestId("pr-task-automation-auto-fix")).not.toBeNull();
    expect(screen.getByTestId("pr-task-automation-auto-merge")).not.toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.11
  it("shows per-PR automation details after the icon disclosure hydrates", async () => {
    const response = Promise.resolve({
      task_prs: { [TASK_ID]: [makePR({ repository_id: "repo-1" })] },
    });
    listTaskPRsMock.mockReturnValue(response);
    getTaskCIAutomationOptionsMock.mockResolvedValue(makeAutomationOptions());
    renderWithStore(
      { workspaces: { items: [], activeId: WORKSPACE_ID } },
      <TaskContributionIcons taskId={TASK_ID} prInfo={{ number: 1, state: "open" }} />,
    );

    fireEvent.pointerEnter(screen.getByTestId(`pr-task-icon-${TASK_ID}`), {
      pointerType: "mouse",
    });
    await act(async () => {
      await response;
    });

    await waitFor(() =>
      expect(screen.getAllByTestId("pr-task-automation-details").length).toBeGreaterThan(0),
    );
    expect(screen.getAllByText("o/r PR #1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Auto-fix").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Auto-merge").length).toBeGreaterThan(0);
  });
});
