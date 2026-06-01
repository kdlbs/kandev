import { afterEach, describe, expect, it } from "vitest";
import type { ReactNode } from "react";
import { cleanup, render } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import { PRTaskIcon } from "./pr-task-icon";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const WS_ID = "ws-1";

// PRTaskIcon reads taskPRs from the TanStack Query cache for the active
// workspace, so seed that cache + the active workspace id (client-only).
function renderWithPRs(prsByTaskId: Record<string, unknown> | undefined, ui: ReactNode) {
  const queryClient = createTestQueryClient();
  if (prsByTaskId) {
    queryClient.setQueryData(qk.github.prs(WS_ID), { task_prs: prsByTaskId });
  }
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={{ workspaces: { activeId: WS_ID } } as Partial<AppState>}>
        <TooltipProvider>{ui}</TooltipProvider>
      </StateProvider>
    </QueryClientProvider>,
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

describe("PRTaskIcon corrupted cache entry", () => {
  // Regression: an upstream payload (partial hydration, WS reorder, etc.) once
  // landed in task_prs["task-1"] as a non-array truthy value. The length-based
  // guards then fell through into MultiPRIcon, where for-of threw `prs is not
  // iterable`. PRTaskIcon must bail rather than crash.
  it("renders nothing when task_prs[taskId] is a non-array object", () => {
    const { container } = renderWithPRs(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      { "task-1": {} as any },
      <PRTaskIcon taskId="task-1" />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when task_prs[taskId] is undefined", () => {
    const { container } = renderWithPRs(undefined, <PRTaskIcon taskId="missing" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders an icon when task_prs[taskId] is a valid array of one PR", () => {
    const { container } = renderWithPRs({ "task-1": [makePR()] }, <PRTaskIcon taskId="task-1" />);
    expect(container.querySelector('[data-testid="pr-task-icon-task-1"]')).not.toBeNull();
  });

  it("renders the multi-PR icon when task_prs[taskId] has multiple PRs", () => {
    const { container } = renderWithPRs(
      {
        "task-1": [
          makePR({ id: "a", repository_id: "repo-a", pr_number: 1 }),
          makePR({ id: "b", repository_id: "repo-b", pr_number: 2 }),
        ],
      },
      <PRTaskIcon taskId="task-1" />,
    );
    const icon = container.querySelector('[data-testid="pr-task-icon-task-1"]');
    expect(icon).not.toBeNull();
    expect(icon?.getAttribute("data-pr-count")).toBe("2");
  });
});
