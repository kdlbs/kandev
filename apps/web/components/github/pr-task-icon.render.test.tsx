import { afterEach, describe, expect, it } from "vitest";
import type { ReactNode } from "react";
import { cleanup, render } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { PRTaskIcon } from "./pr-task-icon";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

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
    task_id: "task-1",
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

afterEach(() => cleanup());

describe("PRTaskIcon corrupted store entry", () => {
  // Regression: an upstream payload (partial hydration, WS reorder, etc.) once
  // landed in taskPRs.byTaskId["task-1"] as a non-array truthy value. The
  // length-based guards then fell through into MultiPRIcon, where for-of
  // threw `prs is not iterable`. PRTaskIcon must bail rather than crash.
  it("renders nothing when byTaskId[taskId] is a non-array object", () => {
    const { container } = renderWithStore(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      { taskPRs: { byTaskId: { "task-1": {} as any } } } as Partial<AppState>,
      <PRTaskIcon taskId="task-1" />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when byTaskId[taskId] is undefined", () => {
    const { container } = renderWithStore(undefined, <PRTaskIcon taskId="missing" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders an icon when byTaskId[taskId] is a valid array of one PR", () => {
    const { container } = renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR()] } } },
      <PRTaskIcon taskId="task-1" />,
    );
    expect(container.querySelector('[data-testid="pr-task-icon-task-1"]')).not.toBeNull();
  });

  it("renders the multi-PR icon when byTaskId[taskId] has multiple PRs", () => {
    const { container } = renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            "task-1": [
              makePR({ id: "a", repository_id: "repo-a", pr_number: 1 }),
              makePR({ id: "b", repository_id: "repo-b", pr_number: 2 }),
            ],
          },
        },
      },
      <PRTaskIcon taskId="task-1" />,
    );
    const icon = container.querySelector('[data-testid="pr-task-icon-task-1"]');
    expect(icon).not.toBeNull();
    expect(icon?.getAttribute("data-pr-count")).toBe("2");
  });
});
