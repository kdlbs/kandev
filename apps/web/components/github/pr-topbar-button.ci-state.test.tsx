import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import { PRTopbarButton } from "./pr-topbar-button";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const TASK_ID = "task-topbar-ci";
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: listTaskPRsMock,
  listWorkspaceTaskPRs: vi.fn().mockResolvedValue({ task_prs: {} }),
}));

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    owner: "acme",
    repo: "widget",
    pr_number: 42,
    pr_url: "https://github.com/acme/widget/pull/42",
    pr_title: "Topbar CI",
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

function renderTopbar(prs: TaskPR[]) {
  const initialState = {
    tasks: { activeTaskId: TASK_ID },
    workspaces: { activeId: WORKSPACE_ID },
    taskPRs: { byTaskId: { [TASK_ID]: prs } },
  } as unknown as Partial<AppState>;
  return render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>
          <PRTopbarButton />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
});

afterEach(() => cleanup());

describe("PRTopbarButton CI state", () => {
  it("shows the CI bubble beside a muted draft topbar glyph", () => {
    renderTopbar([makePR({ mergeable_state: "draft", checks_state: "failure" })]);
    const button = screen.getByTestId("pr-topbar-button");
    const bubble = screen.getByTestId("pr-ci-bubble-failed");
    expect(bubble.parentElement?.className).toContain("text-muted-foreground");
    expect(button.getAttribute("aria-label")).toContain("Checks failed");
  });

  it("aggregates the highest-priority CI state across multiple PRs", () => {
    renderTopbar([
      makePR({ id: "a", pr_number: 42, checks_state: "success" }),
      makePR({ id: "b", pr_number: 43, checks_state: "pending" }),
      makePR({ id: "c", pr_number: 44, checks_state: "failure" }),
    ]);
    expect(screen.getByTestId("pr-ci-bubble-failed")).not.toBeNull();
  });

  it("keeps a passing multi-PR topbar free of bubbles", () => {
    renderTopbar([
      makePR({ id: "a", pr_number: 42, checks_state: "success" }),
      makePR({ id: "b", pr_number: 43, checks_state: "success" }),
    ]);
    expect(screen.queryByTestId("pr-ci-bubble-failed")).toBeNull();
    expect(screen.queryByTestId("pr-ci-bubble-in-progress")).toBeNull();
  });
});
