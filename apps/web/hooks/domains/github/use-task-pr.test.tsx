import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ComponentProps, type ReactNode } from "react";
import { renderHook, cleanup, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import { useTaskPR, upsertTaskPRIntoCaches } from "./use-task-pr";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const WS_ID = "ws-1";

// WS sync is fire-and-forget; no socket in unit tests.
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: vi.fn(() => null),
}));

const listWorkspaceTaskPRs = vi.fn();
vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    getPRFeedback: vi.fn().mockResolvedValue(null),
    listWorkspaceTaskPRs: (...args: unknown[]) => listWorkspaceTaskPRs(...args),
  };
});

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr",
    task_id: "task-1",
    owner: "acme",
    repo: "demo",
    pr_number: 99,
    pr_url: "",
    pr_title: "",
    head_branch: "",
    base_branch: "",
    author_login: "",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 1,
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

const INITIAL_STATE = { workspaces: { activeId: WS_ID } } as Partial<AppState>;

function makeWrapper(queryClient: QueryClient) {
  return function wrapper({ children }: { children: ReactNode }) {
    return createElement(
      QueryClientProvider,
      { client: queryClient },
      createElement(
        StateProvider,
        { initialState: INITIAL_STATE } as ComponentProps<typeof StateProvider>,
        children,
      ),
    );
  };
}

beforeEach(() => {
  listWorkspaceTaskPRs.mockReset();
});

afterEach(() => cleanup());

describe("useTaskPR", () => {
  // Regression: on the mobile layout the desktop board / sidebar (the only
  // callers of useWorkspacePRs) are never mounted, so the qk.github.prs(wsId)
  // query was never created and the chip stayed empty. useTaskPR must fetch
  // the workspace PR query itself so any surface mounting the chip populates
  // the cache.
  it("fetches the workspace PR query when no other consumer has (mobile path)", async () => {
    listWorkspaceTaskPRs.mockResolvedValue({ task_prs: { "task-1": [makePR()] } });
    const queryClient = createTestQueryClient();

    const { result } = renderHook(() => useTaskPR("task-1"), {
      wrapper: makeWrapper(queryClient),
    });

    await waitFor(() => {
      expect(result.current.pr?.pr_number).toBe(99);
    });
    expect(listWorkspaceTaskPRs).toHaveBeenCalledWith(WS_ID, expect.anything());
  });

  it("reads a PR already seeded in the workspace cache without overwriting it", async () => {
    listWorkspaceTaskPRs.mockResolvedValue({ task_prs: {} });
    const queryClient = createTestQueryClient();
    queryClient.setQueryData(qk.github.prs(WS_ID), { task_prs: { "task-1": [makePR()] } });

    const { result } = renderHook(() => useTaskPR("task-1"), {
      wrapper: makeWrapper(queryClient),
    });

    // Seeded data is fresh (staleTime 30s), so the chip reads it immediately.
    expect(result.current.pr?.pr_number).toBe(99);
  });
});

describe("upsertTaskPRIntoCaches", () => {
  // Multi-branch regression: two PRs on the SAME repo (same repository_id) but
  // different pr_number must coexist as siblings. Keying the upsert on
  // repository_id alone collapsed them onto one slot and the second WS event
  // silently overwrote the first.
  it("keeps multiple PRs on the same repo as siblings (keyed by pr_number)", () => {
    const queryClient = createTestQueryClient();
    queryClient.setQueryData(qk.github.prs(WS_ID), { task_prs: {} });

    const first = makePR({ id: "pr-a", repository_id: "repo-1", pr_number: 1 });
    const second = makePR({ id: "pr-b", repository_id: "repo-1", pr_number: 2 });
    upsertTaskPRIntoCaches(queryClient, first);
    upsertTaskPRIntoCaches(queryClient, second);

    const data = queryClient.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(
      qk.github.prs(WS_ID),
    );
    expect(data?.task_prs["task-1"]).toHaveLength(2);
    expect(data?.task_prs["task-1"].map((p) => p.pr_number).sort()).toEqual([1, 2]);
  });

  it("updates an existing PR in place when (repository_id, pr_number) match", () => {
    const queryClient = createTestQueryClient();
    queryClient.setQueryData(qk.github.prs(WS_ID), { task_prs: {} });

    upsertTaskPRIntoCaches(
      queryClient,
      makePR({ id: "pr-a", repository_id: "repo-1", pr_number: 1, state: "open" }),
    );
    upsertTaskPRIntoCaches(
      queryClient,
      makePR({ id: "pr-a", repository_id: "repo-1", pr_number: 1, state: "merged" }),
    );

    const data = queryClient.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(
      qk.github.prs(WS_ID),
    );
    expect(data?.task_prs["task-1"]).toHaveLength(1);
    expect(data?.task_prs["task-1"][0].state).toBe("merged");
  });
});
