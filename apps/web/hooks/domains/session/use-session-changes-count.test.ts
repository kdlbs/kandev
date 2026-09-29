import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { GitStatusEntry, SessionCommit } from "@/lib/state/slices/session-runtime/types";
import { renderSessionRead } from "./session-read-test-helpers";
import { useSessionChangesCount } from "./use-session-changes-count";

type FileEntry = { path: string; status: "modified"; staged: boolean };
type StoreOptions = {
  envBySession?: Record<string, string>;
  byEnvironmentRepo?: Record<string, Record<string, GitStatusEntry>>;
  commitsByEnvironmentId?: Record<string, SessionCommit[]>;
};
let initial: StoreOptions = {};
function setStore(options: StoreOptions) {
  initial = options;
}
function renderCount(sessionId: string | null) {
  return renderSessionRead(useSessionChangesCount, sessionId, (store) => {
    store.setState({
      environmentIdBySessionId: initial.envBySession ?? {},
      gitStatus: { byEnvironmentId: {}, byEnvironmentRepo: initial.byEnvironmentRepo ?? {} },
      sessionCommits: {
        ...store.getState().sessionCommits,
        byEnvironmentId: initial.commitsByEnvironmentId ?? {},
      },
      connection: { status: "disconnected", error: null, issueSeverity: "none" },
    });
  });
}
function commit(sha: string): SessionCommit {
  return {
    id: sha,
    session_id: "sess-1",
    commit_sha: sha,
    parent_sha: "parent",
    author_name: "Test",
    author_email: "test@example.test",
    commit_message: sha,
    committed_at: "2026-09-28T12:00:00Z",
    created_at: "2026-09-28T12:00:00Z",
    files_changed: 1,
    insertions: 1,
    deletions: 0,
  };
}

function file(path: string): FileEntry {
  return { path, status: "modified", staged: false };
}

function status(files: string[], repository_name?: string): GitStatusEntry {
  const map: Record<string, FileEntry> = {};
  for (const p of files) map[p] = file(p);
  return {
    branch: "feature",
    remote_branch: null,
    modified: files,
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: map,
    timestamp: "t",
    repository_name,
  };
}

describe("useSessionChangesCount", () => {
  beforeEach(() => {
    setWebSocketClient(null);
    setStore({});
  });

  afterEach(() => {
    cleanup();
  });

  it("returns 0 when the session has no gitStatus and no commits yet", () => {
    const { result } = renderCount("sess-new");
    expect(result.current.value).toBe(0);
  });

  it("returns 0 for null session id", () => {
    const { result } = renderCount(null);
    expect(result.current.value).toBe(0);
  });

  it("counts files from a single-repo workspace via the empty repo key", () => {
    setStore({
      envBySession: { "sess-1": "env-1" },
      byEnvironmentRepo: { "env-1": { "": status(["a.ts", "b.ts"]) } },
    });
    const { result } = renderCount("sess-1");
    expect(result.current.value).toBe(2);
  });

  it("sums files across every repo in a multi-repo workspace", () => {
    // Reproduces the reported bug shape: useSessionGitStatus alone would
    // report only the last-arriving repo's files (1), masking changes in
    // sibling repos. The aggregated count must show the true total.
    setStore({
      envBySession: { "sess-1": "env-1" },
      byEnvironmentRepo: {
        "env-1": {
          frontend: status(["app.tsx", "page.tsx"], "frontend"),
          backend: status(["server.go"], "backend"),
        },
      },
    });
    const { result } = renderCount("sess-1");
    expect(result.current.value).toBe(3);
  });

  it("includes commits in the total count", () => {
    setStore({
      envBySession: { "sess-1": "env-1" },
      byEnvironmentRepo: { "env-1": { "": status(["a.ts"]) } },
      commitsByEnvironmentId: { "env-1": [commit("x"), commit("y")] },
    });
    const { result } = renderCount("sess-1");
    expect(result.current.value).toBe(3);
  });

  it("falls back to sessionId when no environment mapping is registered yet", () => {
    setStore({
      byEnvironmentRepo: { "sess-pending": { "": status(["only.ts"]) } },
    });
    const { result } = renderCount("sess-pending");
    expect(result.current.value).toBe(1);
  });

  it("does not leak stale data from a different session's environment", () => {
    // The bug: a brand-new task whose env has no gitStatus yet must not pick
    // up another task's count just because their env IDs happen to share the
    // map. The selector keys strictly by envKey resolved from sessionId.
    setStore({
      envBySession: { "sess-old": "env-old", "sess-new": "env-new" },
      byEnvironmentRepo: { "env-old": { "": status(["leak.ts", "leak2.ts"]) } },
      commitsByEnvironmentId: { "env-old": [commit("old")] },
    });
    const { result } = renderCount("sess-new");
    expect(result.current.value).toBe(0);
  });
});
