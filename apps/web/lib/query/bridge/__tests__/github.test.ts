import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerGithubBridge } from "../github";
import { qk } from "@/lib/query/keys";
import type { TaskPR, GitHubStatus, GitHubRateLimitSnapshot } from "@/lib/types/github";

// ---------------------------------------------------------------------------
// Fake WS client — captures handlers registered via ws.on()
// ---------------------------------------------------------------------------
type Handler = (message: { payload: unknown }) => void;

interface FakeWs {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  on: ReturnType<typeof vi.fn<any[], any>>;
  emit: (type: string, payload: unknown) => void;
  listeners: Map<string, Set<Handler>>;
}

function createFakeWs(): FakeWs {
  const listeners = new Map<string, Set<Handler>>();

  const on = vi.fn((type: string, handler: Handler) => {
    const set = listeners.get(type) ?? new Set();
    set.add(handler);
    listeners.set(type, set);
    return () => {
      listeners.get(type)?.delete(handler);
    };
  });

  function emit(type: string, payload: unknown) {
    for (const handler of listeners.get(type) ?? []) {
      handler({ payload } as { payload: unknown });
    }
  }

  return { on, emit, listeners };
}

function createTestClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
}

const WORKSPACE_ID = "ws-1";
const TASK_ID = "task-1";
const REPO_A = "repo-a";
const REPO_B = "repo-b";
const TIMESTAMP = "2026-05-24T10:00:00Z";
const FUTURE_RESET = "2030-01-01T00:00:00Z";
const WS_TASK_PR_UPDATED = "github.task_pr.updated";
const WS_RATE_LIMIT_UPDATED = "github.rate_limit.updated";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
    task_id: TASK_ID,
    owner: "octocat",
    repo: "hello-world",
    pr_number: 42,
    pr_url: "https://github.com/octocat/hello-world/pull/42",
    pr_title: "Test PR",
    head_branch: "feat/test",
    base_branch: "main",
    author_login: "octocat",
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
    additions: 10,
    deletions: 5,
    created_at: TIMESTAMP,
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

function makeStatus(overrides: Partial<GitHubStatus> = {}): GitHubStatus {
  return {
    authenticated: true,
    username: "octocat",
    auth_method: "pat",
    token_configured: true,
    required_scopes: ["repo"],
    ...overrides,
  };
}

function makeRateLimitSnapshot(
  resource: "core" | "graphql" | "search",
  remaining: number,
): GitHubRateLimitSnapshot {
  return {
    resource,
    remaining,
    limit: 5000,
    reset_at: FUTURE_RESET,
    updated_at: TIMESTAMP,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("registerGithubBridge", () => {
  let ws: FakeWs;
  let qc: QueryClient;
  let cleanup: () => void;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    cleanup = registerGithubBridge(ws as never, qc);
  });

  it("returns a cleanup function", () => {
    expect(typeof cleanup).toBe("function");
  });

  it("registers handlers for github.task_pr.updated and github.rate_limit.updated", () => {
    expect(ws.on).toHaveBeenCalledWith(WS_TASK_PR_UPDATED, expect.any(Function));
    expect(ws.on).toHaveBeenCalledWith(WS_RATE_LIMIT_UPDATED, expect.any(Function));
  });

  it("cleanup returns unsubscribe functions that do not throw", () => {
    expect(() => cleanup()).not.toThrow();
  });
});

describe("registerGithubBridge — github.task_pr.updated", () => {
  let ws: FakeWs;
  let qc: QueryClient;
  let cleanup: () => void;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    cleanup = registerGithubBridge(ws as never, qc);

    // Seed workspace PR cache
    qc.setQueryData(qk.github.prs(WORKSPACE_ID), {
      task_prs: {} as Record<string, TaskPR[]>,
    });
  });

  afterEach(() => cleanup());

  it("upserts a new PR into an existing workspace cache", () => {
    const pr = makePR({ repository_id: REPO_A });
    ws.emit(WS_TASK_PR_UPDATED, pr);

    const data = qc.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(
      qk.github.prs(WORKSPACE_ID),
    );
    expect(data?.task_prs[TASK_ID]).toHaveLength(1);
    expect(data?.task_prs[TASK_ID][0].id).toBe("pr-1");
  });

  it("updates an existing PR by repository_id (upsert semantics)", () => {
    const prA = makePR({ id: "pr-a", repository_id: REPO_A, additions: 10 });
    qc.setQueryData(qk.github.prs(WORKSPACE_ID), {
      task_prs: { [TASK_ID]: [prA] },
    });

    const prAUpdated = makePR({ id: "pr-a", repository_id: REPO_A, additions: 99 });
    ws.emit(WS_TASK_PR_UPDATED, prAUpdated);

    const data = qc.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(
      qk.github.prs(WORKSPACE_ID),
    );
    expect(data?.task_prs[TASK_ID]).toHaveLength(1);
    expect(data?.task_prs[TASK_ID][0].additions).toBe(99);
  });

  it("appends a second PR with a different repository_id (multi-repo support)", () => {
    const prA = makePR({ id: "pr-a", repository_id: REPO_A });
    qc.setQueryData(qk.github.prs(WORKSPACE_ID), {
      task_prs: { [TASK_ID]: [prA] },
    });

    const prB = makePR({ id: "pr-b", repository_id: REPO_B, pr_number: 43 });
    ws.emit(WS_TASK_PR_UPDATED, prB);

    const data = qc.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(
      qk.github.prs(WORKSPACE_ID),
    );
    expect(data?.task_prs[TASK_ID]).toHaveLength(2);
  });

  it("is a no-op when task_id is missing from the payload", () => {
    qc.setQueryData(qk.github.prs(WORKSPACE_ID), {
      task_prs: {} as Record<string, TaskPR[]>,
    });
    const setQueryDataSpy = vi.spyOn(qc, "setQueryData");

    const prWithoutTaskId = { ...makePR(), task_id: "" };
    ws.emit(WS_TASK_PR_UPDATED, prWithoutTaskId);

    // setQueryData should not have been called (event skipped)
    expect(setQueryDataSpy).not.toHaveBeenCalled();
  });
});

describe("registerGithubBridge — github.rate_limit.updated", () => {
  let ws: FakeWs;
  let qc: QueryClient;
  let cleanup: () => void;

  beforeEach(() => {
    ws = createFakeWs();
    qc = createTestClient();
    cleanup = registerGithubBridge(ws as never, qc);
  });

  afterEach(() => cleanup());

  it("applies rate-limit snapshots into the GitHub status cache", () => {
    qc.setQueryData(qk.github.status(), makeStatus());

    ws.emit(WS_RATE_LIMIT_UPDATED, {
      trigger: "graphql",
      snapshots: [makeRateLimitSnapshot("graphql", 0)],
    });

    const status = qc.getQueryData<GitHubStatus>(qk.github.status());
    expect(status?.rate_limit?.graphql?.remaining).toBe(0);
  });

  it("merges multiple snapshots in one update", () => {
    qc.setQueryData(qk.github.status(), makeStatus());

    ws.emit(WS_RATE_LIMIT_UPDATED, {
      trigger: "core",
      snapshots: [makeRateLimitSnapshot("core", 4500), makeRateLimitSnapshot("graphql", 100)],
    });

    const status = qc.getQueryData<GitHubStatus>(qk.github.status());
    expect(status?.rate_limit?.core?.remaining).toBe(4500);
    expect(status?.rate_limit?.graphql?.remaining).toBe(100);
  });

  it("only patches updated resources, leaving others untouched", () => {
    qc.setQueryData(
      qk.github.status(),
      makeStatus({
        rate_limit: { core: makeRateLimitSnapshot("core", 4500) },
      }),
    );

    ws.emit(WS_RATE_LIMIT_UPDATED, {
      trigger: "graphql",
      snapshots: [makeRateLimitSnapshot("graphql", 200)],
    });

    const status = qc.getQueryData<GitHubStatus>(qk.github.status());
    expect(status?.rate_limit?.core?.remaining).toBe(4500); // untouched
    expect(status?.rate_limit?.graphql?.remaining).toBe(200);
  });

  it("is a no-op when status is not yet cached", () => {
    // No status seeded — the updater should return undefined without throwing.
    expect(() => {
      ws.emit(WS_RATE_LIMIT_UPDATED, {
        trigger: "core",
        snapshots: [makeRateLimitSnapshot("core", 0)],
      });
    }).not.toThrow();

    expect(qc.getQueryData(qk.github.status())).toBeUndefined();
  });

  it("is a no-op when snapshots array is empty", () => {
    qc.setQueryData(qk.github.status(), makeStatus());

    ws.emit(WS_RATE_LIMIT_UPDATED, {
      trigger: "core",
      snapshots: [],
    });

    // Status should be unchanged — bridge ignores empty snapshots array.
    const status = qc.getQueryData<GitHubStatus>(qk.github.status());
    expect(status?.rate_limit).toBeUndefined();
  });
});
