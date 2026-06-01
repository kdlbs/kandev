import { describe, it, expect, vi, beforeEach } from "vitest";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { registerSessionRuntimeBridge } from "../session-runtime";
import { qk } from "@/lib/query/keys";
import type { GitStatusData } from "../session-runtime-types";
import type { GitStatusEntry, TodoEntry } from "@/lib/state/slices/session-runtime/types";

/**
 * Builds a git status with the given modified files. Both `modified` (the
 * legacy string list) and `files` (the FileInfo map used by the bridge's
 * hasGitStatusChanged change-detection guard) are populated so subsequent
 * updates register as meaningful changes.
 */
function entry(overrides: { modified: string[]; repository_name?: string }): GitStatusEntry {
  const files: GitStatusEntry["files"] = {};
  for (const path of overrides.modified) {
    files[path] = { path, status: "modified", staged: false };
  }
  return {
    branch: "main",
    remote_branch: null,
    modified: overrides.modified,
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files,
    timestamp: new Date().toISOString() + Math.random(),
    repository_name: overrides.repository_name,
  };
}

const REPO_FRONTEND = "frontend";
const REPO_BACKEND = "backend";
const FRONTEND_FILE = "frontend.tsx";
const BACKEND_FILE = "backend.go";

// ---------------------------------------------------------------------------
// Mock for invalidateCumulativeDiffCache
// ---------------------------------------------------------------------------
vi.mock("@/hooks/domains/session/use-cumulative-diff", () => ({
  invalidateCumulativeDiffCache: vi.fn(),
}));

// ---------------------------------------------------------------------------
// WS event name constants
// ---------------------------------------------------------------------------

const GIT_EVENT = "session.git.event";

// ---------------------------------------------------------------------------
// Fake WebSocket client
// ---------------------------------------------------------------------------
type Handler = (message: { payload: Record<string, unknown>; timestamp?: string }) => void;

function makeFakeWs() {
  const handlers = new Map<string, Set<Handler>>();
  return {
    on(event: string, handler: Handler) {
      let set = handlers.get(event);
      if (!set) {
        set = new Set();
        handlers.set(event, set);
      }
      set.add(handler);
      return () => set?.delete(handler);
    },
    emit(event: string, payload: Record<string, unknown>, timestamp?: string) {
      const set = handlers.get(event);
      if (!set) return;
      for (const h of set) h({ payload, timestamp });
    },
  };
}

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

type TestContext = {
  ws: ReturnType<typeof makeFakeWs>;
  qc: ReturnType<typeof createTestQueryClient>;
  cleanup: () => void;
};

function makeContext(): TestContext {
  const ws = makeFakeWs();
  const qc = createTestQueryClient();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const cleanup = registerSessionRuntimeBridge(ws as any, qc, (sid) => sid);
  return { ws, qc, cleanup };
}

// ---------------------------------------------------------------------------
// Git event tests
// ---------------------------------------------------------------------------

describe("session-runtime bridge — git events", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("stores git status update into the git cache key", () => {
    const gitStatus = {
      branch: "main",
      remote_branch: "origin/main",
      modified: [],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      files: {},
      timestamp: "2024-01-01T00:00:00Z",
    };

    ctx.ws.emit(GIT_EVENT, { session_id: "sess-1", type: "status_update", status: gitStatus });

    const data = ctx.qc.getQueryData<GitStatusData>(qk.session.git("sess-1"));
    expect(data?.byEnvironmentId?.branch).toBe("main");
    expect(data?.byEnvironmentRepo[""]?.branch).toBe("main");
  });

  it("resolves envKey via getEnvKey resolver for git events", () => {
    const ws2 = makeFakeWs();
    const qc2 = createTestQueryClient();
    const customResolver = (sid: string) => `env-${sid}`;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const localCleanup = registerSessionRuntimeBridge(ws2 as any, qc2, customResolver);

    ws2.emit(GIT_EVENT, {
      session_id: "sess-2",
      type: "status_update",
      status: {
        branch: "dev",
        remote_branch: null,
        modified: [],
        added: [],
        deleted: [],
        untracked: [],
        renamed: [],
        ahead: 0,
        behind: 0,
        files: {},
        timestamp: "t",
      },
    });

    expect(qc2.getQueryData(qk.session.git("env-sess-2"))).toBeTruthy();
    expect(qc2.getQueryData(qk.session.git("sess-2"))).toBeUndefined();
    localCleanup();
  });

  it("prepends commits on commit_created event", () => {
    const commit = {
      id: "c1",
      commit_sha: "abc123",
      parent_sha: "xyz000",
      commit_message: "feat: add thing",
      author_name: "Alice",
      author_email: "alice@example.com",
      files_changed: 3,
      insertions: 10,
      deletions: 2,
      committed_at: "2024-01-01T00:00:00Z",
      created_at: "2024-01-01T00:00:01Z",
    };

    ctx.ws.emit(GIT_EVENT, { session_id: "sess-3", type: "commit_created", commit });

    const commits = ctx.qc.getQueryData(qk.session.commits("sess-3"));
    expect(Array.isArray(commits)).toBe(true);
    expect((commits as unknown[]).length).toBe(1);
  });

  it("clears commits on commits_reset event", () => {
    ctx.qc.setQueryData(qk.session.commits("sess-4"), [{ id: "c1" }]);
    ctx.ws.emit(GIT_EVENT, { session_id: "sess-4", type: "commits_reset" });
    expect(ctx.qc.getQueryData(qk.session.commits("sess-4"))).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Git multi-repo routing tests (ported from the removed Zustand slice test
// lib/state/slices/session-runtime/git-status-multi-repo.test.ts; the per-repo
// routing + change-detection guard now live in the bridge's updateGitStatusData).
// ---------------------------------------------------------------------------

describe("session-runtime bridge — git multi-repo routing", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("routes a single status without repository_name into the empty-key repo entry", () => {
    ctx.ws.emit(GIT_EVENT, {
      session_id: "sess-mr0",
      type: "status_update",
      status: entry({ modified: ["a.ts"] }),
    });
    const data = ctx.qc.getQueryData<GitStatusData>(qk.session.git("sess-mr0"));
    expect(data?.byEnvironmentId?.modified).toEqual(["a.ts"]);
    // Per-repo map gets an entry under the empty key (mirrors single-repo) so
    // consumers that only read byEnvironmentRepo still see it.
    expect(Object.keys(data?.byEnvironmentRepo ?? {})).toEqual([""]);
  });

  it("routes per-repo statuses into byEnvironmentRepo keyed by repository_name", () => {
    ctx.ws.emit(GIT_EVENT, {
      session_id: "sess-mr1",
      type: "status_update",
      status: entry({ modified: [FRONTEND_FILE], repository_name: REPO_FRONTEND }),
    });
    ctx.ws.emit(GIT_EVENT, {
      session_id: "sess-mr1",
      type: "status_update",
      status: entry({ modified: [BACKEND_FILE], repository_name: REPO_BACKEND }),
    });

    const repoMap =
      ctx.qc.getQueryData<GitStatusData>(qk.session.git("sess-mr1"))?.byEnvironmentRepo ?? {};
    expect(Object.keys(repoMap).sort()).toEqual([REPO_BACKEND, REPO_FRONTEND]);
    expect(repoMap[REPO_FRONTEND].modified).toEqual([FRONTEND_FILE]);
    expect(repoMap[REPO_BACKEND].modified).toEqual([BACKEND_FILE]);
  });

  it("does NOT overwrite the per-repo map when a sibling repo updates", () => {
    ctx.ws.emit(GIT_EVENT, {
      session_id: "sess-mr2",
      type: "status_update",
      status: entry({ modified: [FRONTEND_FILE], repository_name: REPO_FRONTEND }),
    });
    ctx.ws.emit(GIT_EVENT, {
      session_id: "sess-mr2",
      type: "status_update",
      status: entry({ modified: [BACKEND_FILE], repository_name: REPO_BACKEND }),
    });
    // Update frontend again — backend must still be there.
    ctx.ws.emit(GIT_EVENT, {
      session_id: "sess-mr2",
      type: "status_update",
      status: entry({ modified: ["frontend2.tsx"], repository_name: REPO_FRONTEND }),
    });

    const repoMap =
      ctx.qc.getQueryData<GitStatusData>(qk.session.git("sess-mr2"))?.byEnvironmentRepo ?? {};
    expect(repoMap[REPO_FRONTEND].modified).toEqual(["frontend2.tsx"]);
    expect(repoMap[REPO_BACKEND].modified).toEqual([BACKEND_FILE]);
  });
});

// ---------------------------------------------------------------------------
// Session data event tests
// ---------------------------------------------------------------------------

describe("session-runtime bridge — session data events", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("stores todos in the TQ cache", () => {
    ctx.ws.emit("session.todos_updated", {
      session_id: "sess-5",
      entries: [
        { description: "write tests", status: "in_progress", priority: "high" },
        { description: "ship", status: "pending" },
      ],
    });

    const todos = ctx.qc.getQueryData<TodoEntry[]>(qk.session.todos("sess-5"));
    expect(todos).toHaveLength(2);
    expect(todos?.[0].description).toBe("write tests");
    expect(todos?.[0].status).toBe("in_progress");
  });

  it("stores session mode change in TQ cache", () => {
    ctx.ws.emit("session.mode_changed", {
      session_id: "sess-6",
      current_mode_id: "plan",
      available_modes: [{ id: "plan", name: "Plan Mode", description: "Planning" }],
    });

    const mode = ctx.qc.getQueryData(["session", "sess-6", "mode"]);
    expect(mode).toMatchObject({ currentModeId: "plan" });
  });

  it("stores prompt usage in TQ cache", () => {
    ctx.ws.emit("session.prompt_usage", {
      session_id: "sess-7",
      usage: {
        input_tokens: 100,
        output_tokens: 50,
        cached_read_tokens: 20,
        cached_write_tokens: 10,
        total_tokens: 150,
      },
    });

    const usage = ctx.qc.getQueryData(["session", "sess-7", "promptUsage"]);
    expect(usage).toMatchObject({ inputTokens: 100, outputTokens: 50, totalTokens: 150 });
  });

  it("stores valid poll mode in TQ cache", () => {
    ctx.ws.emit("session.poll_mode_changed", { session_id: "sess-8", poll_mode: "fast" });
    expect(ctx.qc.getQueryData(["session", "sess-8", "pollMode"])).toBe("fast");
  });

  it("ignores invalid poll mode values", () => {
    ctx.ws.emit("session.poll_mode_changed", { session_id: "sess-9", poll_mode: "turbo" });
    expect(ctx.qc.getQueryData(["session", "sess-9", "pollMode"])).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Prepare progress tests
// ---------------------------------------------------------------------------

describe("session-runtime bridge — prepare progress", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("stores prepare progress steps in TQ cache", () => {
    ctx.ws.emit("executor.prepare.progress", {
      session_id: "sess-10",
      step_index: 0,
      step_name: "Install deps",
      step_command: "npm install",
      status: "running",
    });

    const progress = ctx.qc.getQueryData(["session", "sess-10", "prepareProgress"]);
    expect(progress).toMatchObject({ status: "preparing" });
  });

  it("stores prepare completed in TQ cache", () => {
    ctx.ws.emit("executor.prepare.completed", {
      session_id: "sess-11",
      success: true,
      duration_ms: 5000,
    });

    const progress = ctx.qc.getQueryData(["session", "sess-11", "prepareProgress"]);
    expect(progress).toMatchObject({ status: "completed", durationMs: 5000 });
  });

  it("cleanup removes handlers", () => {
    ctx.cleanup();
    ctx.ws.emit("session.todos_updated", {
      session_id: "after-cleanup",
      entries: [{ description: "should not appear", status: "pending" }],
    });
    expect(ctx.qc.getQueryData(qk.session.todos("after-cleanup"))).toBeUndefined();
  });
});
