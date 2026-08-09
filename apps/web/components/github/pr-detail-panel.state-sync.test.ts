import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { createElement, useEffect } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { afterEach, describe, it, expect, vi } from "vitest";
import { PRDetailContent } from "./pr-detail-panel";
import type { TaskPR, PRFeedback, GitHubPR } from "@/lib/types/github";

const feedbackMocks = vi.hoisted(() => ({
  refresh: vi.fn(),
  value: null as PRFeedback | null,
}));

vi.mock("@/hooks/domains/github/use-pr-feedback", () => ({
  usePRFeedback: () => ({
    feedback: feedbackMocks.value,
    loading: false,
    refresh: feedbackMocks.refresh,
  }),
}));
vi.mock("@/lib/state/slices/comments", () => ({
  isPRFeedbackComment: () => false,
  useCommentsStore: (
    selector: (state: {
      addComment: () => void;
      pendingForChat: string[];
      byId: object;
    }) => unknown,
  ) => selector({ addComment: vi.fn(), pendingForChat: [], byId: {} }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
}));
vi.mock("./pr-merge-button", () => ({ PRMergeButton: () => null }));
vi.mock("./pr-mergeability-notice", () => ({
  PRMergeabilityNotice: () => null,
  buildConflictResolutionMessage: () => "",
}));
vi.mock("./pr-checks-section", () => ({ ChecksSection: () => null }));
vi.mock("./pr-comments-section", () => ({ CommentsSection: () => null }));

afterEach(() => {
  cleanup();
  feedbackMocks.refresh.mockReset();
  feedbackMocks.value = null;
});

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task",
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

function makeFeedback(pr: Partial<GitHubPR> = {}): PRFeedback {
  return {
    pr: {
      number: 1,
      title: "Test PR",
      url: "",
      html_url: "",
      state: "open",
      head_branch: "feat",
      base_branch: "main",
      author_login: "alice",
      repo_owner: "o",
      repo_name: "r",
      draft: false,
      mergeable: true,
      additions: 0,
      deletions: 0,
      ...pr,
    } as GitHubPR,
    reviews: [],
    comments: [],
    checks: [],
    has_issues: false,
  };
}

const TEST_INITIAL_STATE = {
  workspaces: { activeId: "workspace-1" },
} as unknown as Partial<AppState>;

// Seeds the store the way the boot payload / WS sync would, so the probe below
// starts from a real row rather than an empty map.
function SeedTaskPR({ taskPR }: { taskPR: TaskPR }) {
  const setTaskPRs = useAppStore((s) => s.setTaskPRs);
  useEffect(() => {
    setTaskPRs({ [taskPR.task_id]: [taskPR] });
  }, [setTaskPRs, taskPR]);
  return null;
}

function renderPRDetail(taskPR: TaskPR) {
  return render(
    createElement(StateProvider, {
      initialState: TEST_INITIAL_STATE,
      children: createElement(
        TooltipProvider,
        undefined,
        createElement(PRDetailContent, { taskPR, sessionId: "session-1" }),
      ),
    }),
  );
}

// Renders the panel alongside a probe that reports the live store row, so a
// test can assert what the panel did (or did not) write back into it.
function renderPRDetailWithStoreProbe(taskPR: TaskPR) {
  let read: () => TaskPR | undefined = () => undefined;
  function Probe() {
    const byTaskId = useAppStore((s) => s.taskPRs.byTaskId);
    read = () => byTaskId[taskPR.task_id]?.[0];
    return null;
  }
  const view = render(
    createElement(StateProvider, {
      initialState: TEST_INITIAL_STATE,
      children: createElement(
        TooltipProvider,
        undefined,
        createElement(SeedTaskPR, { taskPR }),
        createElement(Probe),
        createElement(PRDetailContent, { taskPR, sessionId: "session-1" }),
      ),
    }),
  );
  return { ...view, readStoreRow: () => read() };
}

describe("PRDetailContent merged state", () => {
  // The panel reads live state straight off the feedback response, so a merge
  // observed by the fetch paints immediately.
  it("renders the merged badge from live feedback", () => {
    feedbackMocks.value = makeFeedback({ state: "merged" });
    renderPRDetail(makeTaskPR({ state: "open" }));
    expect(screen.getByText("merged")).toBeTruthy();
  });

  // And it keeps rendering merged once the row itself has converged — which is
  // what survives a reload, since the store rehydrates from github_task_prs.
  it("renders the merged badge from a converged store row before feedback loads", () => {
    feedbackMocks.value = null;
    renderPRDetail(makeTaskPR({ state: "merged", merged_at: "2026-01-01T00:00:00Z" }));
    expect(screen.getByText("merged")).toBeTruthy();
  });

  // Regression guard for the collapse: the panel used to patch state /
  // merged_at / closed_at / additions / deletions / mergeable_state into the
  // Zustand row itself (useSyncLivePRState). That write never reached the DB,
  // so this tab went purple while the kanban card stayed green and a reload
  // undid it. The backend now persists during the feedback fetch and the
  // github.task_pr.updated handler applies it — the panel must stay read-only.
  it("does not write live PR state back into the store", async () => {
    feedbackMocks.value = makeFeedback({
      state: "merged",
      merged_at: "2026-01-01T00:00:00Z",
      additions: 42,
      deletions: 7,
    });
    const stale = makeTaskPR({ state: "open", additions: 0, deletions: 0 });
    const { readStoreRow } = renderPRDetailWithStoreProbe(stale);

    await waitFor(() => expect(readStoreRow()).toBeTruthy());
    const row = readStoreRow();
    expect(row?.state).toBe("open");
    expect(row?.merged_at).toBeNull();
    expect(row?.additions).toBe(0);
    expect(row?.deletions).toBe(0);
  });
});
