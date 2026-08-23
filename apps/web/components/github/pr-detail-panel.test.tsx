import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { AppState } from "@/lib/state/store";
import type { PRFeedback, TaskPR } from "@/lib/types/github";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import { PRDetailPanelComponent } from "./pr-detail-panel";

const BOT_COMMENTS_LABEL = "Bot comments (1)";

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    getPRFeedback: vi.fn(),
  };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  setPanelTitle: vi.fn(),
}));

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: overrides.id ?? "pr-id",
    task_id: "task-1",
    owner: "acme",
    repo: "demo",
    pr_number: 1,
    pr_url: "https://github.com/acme/demo/pull/1",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 1,
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

function makeFeedback(pr: TaskPR, botCommentBody: string, state = "open"): PRFeedback {
  return {
    pr: {
      number: pr.pr_number,
      title: pr.pr_title,
      url: pr.pr_url,
      html_url: pr.pr_url,
      state,
      head_branch: pr.head_branch,
      base_branch: pr.base_branch,
      author_login: pr.author_login,
      repo_owner: pr.owner,
      repo_name: pr.repo,
      draft: false,
      mergeable: true,
      additions: 0,
      deletions: 0,
      requested_reviewers: [],
      created_at: "",
      updated_at: "",
      merged_at: null,
      closed_at: null,
    },
    reviews: [],
    comments: [
      {
        id: pr.pr_number,
        author: "codex-bot",
        author_avatar: "",
        author_is_bot: true,
        body: botCommentBody,
        path: "",
        line: 0,
        side: "",
        comment_type: "issue",
        created_at: "",
        updated_at: "",
        in_reply_to: null,
      },
    ],
    checks: [],
    has_issues: false,
  };
}

function StoreCapture({ onReady }: { onReady: (api: StoreApi<AppState>) => void }) {
  const api = useAppStoreApi();
  onReady(api);
  return null;
}

function renderPanel(initialState: Partial<AppState>) {
  let storeApi!: StoreApi<AppState>;
  render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>
          <StoreCapture onReady={(api) => (storeApi = api)} />
          <PRDetailPanelComponent panelId="pr-detail" />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return { getStoreApi: () => storeApi };
}

function taskState(taskId: string, sessionId: string, prs: TaskPR[]): Partial<AppState> {
  return {
    workspaces: { items: [], activeId: "ws-1" },
    taskPRs: { byTaskId: { [taskId]: prs } },
    tasks: {
      activeTaskId: taskId,
      activeSessionId: sessionId,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
  } as unknown as Partial<AppState>;
}

beforeEach(() => {
  vi.mocked(getPRFeedback).mockReset();
});

afterEach(() => {
  cleanup();
});

describe("PRDetailPanelComponent — task switch on the singleton legacy panel", () => {
  it("resets the bot-comments toggle instead of leaking it across a task switch", async () => {
    const pr1 = makePR({ id: "pr-1", task_id: "task-1", pr_number: 1 });
    const pr2 = makePR({ id: "pr-2", task_id: "task-2", pr_number: 2 });

    vi.mocked(getPRFeedback).mockImplementation((_ws, _owner, _repo, prNumber) =>
      Promise.resolve(
        prNumber === 1
          ? makeFeedback(pr1, "PR 1 bot comment")
          : makeFeedback(pr2, "PR 2 bot comment"),
      ),
    );

    const { getStoreApi } = renderPanel(taskState("task-1", "session-1", [pr1]));

    // Task 1: expand the bot-comments disclosure.
    await waitFor(() => expect(screen.getByText(BOT_COMMENTS_LABEL)).toBeTruthy());
    fireEvent.click(screen.getByText(BOT_COMMENTS_LABEL));
    await waitFor(() => expect(screen.getByText("PR 1 bot comment")).toBeTruthy());

    // Switch to a different task whose PR also has exactly one bot comment —
    // the singleton "pr-detail" panel is reused, not remounted by dockview.
    await act(async () => {
      getStoreApi().setState((prev) => ({
        ...prev,
        taskPRs: { byTaskId: { ...prev.taskPRs.byTaskId, "task-2": [pr2] } },
        tasks: { ...prev.tasks, activeTaskId: "task-2", activeSessionId: "session-2" },
      }));
    });

    await waitFor(() => expect(screen.getByText(BOT_COMMENTS_LABEL)).toBeTruthy());
    // Task 2's bot comment must stay collapsed by default, not inherit task 1's
    // expanded disclosure state.
    expect(screen.queryByText("PR 2 bot comment")).toBeNull();
    expect(screen.queryByText("PR 1 bot comment")).toBeNull();
  });

  it("does not show a stale queue notice after live feedback closes the PR", async () => {
    const pr = makePR({
      merge_queue_state: "queued",
      merge_queue_position: 2,
      merge_queue_estimated_time_to_merge_seconds: 120,
    });
    vi.mocked(getPRFeedback).mockResolvedValue(makeFeedback(pr, "Closed PR", "closed"));

    renderPanel(taskState("task-1", "session-1", [pr]));

    await waitFor(() => expect(screen.getByText(BOT_COMMENTS_LABEL)).toBeTruthy());
    expect(screen.queryByTestId("pr-merge-queue-status")).toBeNull();
  });
});
