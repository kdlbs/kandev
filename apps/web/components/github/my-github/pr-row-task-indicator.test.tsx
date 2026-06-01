import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
}));

import { StateProvider } from "@/components/state-provider";
import { PRRowTaskIndicator } from "./pr-row-task-indicator";
import type { TaskPR } from "@/lib/types/github";

function renderWithStore(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <StateProvider>
        <TooltipProvider>{ui}</TooltipProvider>
      </StateProvider>
    </QueryClientProvider>,
  );
}

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
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

describe("PRRowTaskIndicator", () => {
  it("shows 'No task created yet' when tasks is undefined", () => {
    renderWithStore(<PRRowTaskIndicator tasks={undefined} />);
    expect(screen.getByText("No task created yet")).toBeTruthy();
  });

  it("shows 'No task created yet' when tasks is empty", () => {
    renderWithStore(<PRRowTaskIndicator tasks={[]} />);
    expect(screen.getByText("No task created yet")).toBeTruthy();
  });

  it("renders a clickable button with task title for a single task", () => {
    renderWithStore(<PRRowTaskIndicator tasks={[makeTaskPR({ pr_title: "My Feature PR" })]} />);
    const btn = screen.getByRole("button");
    expect(btn.textContent).toContain("My Feature PR");
  });

  it("shows 'Tasks' trigger with count badge for multiple tasks", () => {
    const tasks = [
      makeTaskPR({ id: "a", task_id: "t1", pr_title: "First PR" }),
      makeTaskPR({ id: "b", task_id: "t2", pr_title: "Second PR" }),
    ];
    const { container } = renderWithStore(<PRRowTaskIndicator tasks={tasks} />);
    expect(screen.getByRole("button").textContent).toContain("Tasks");
    expect(container.textContent).toContain("2");
  });

  it("truncates long task titles (>40 chars)", () => {
    const longTitle = "This is an extremely long pull request title that should be truncated";
    renderWithStore(<PRRowTaskIndicator tasks={[makeTaskPR({ pr_title: longTitle })]} />);
    const btn = screen.getByRole("button");
    expect(btn.textContent).toContain("…");
    expect(btn.textContent!.length).toBeLessThan(longTitle.length + 5);
  });
});
