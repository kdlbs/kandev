import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

let storeState: Record<string, unknown> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
}));

import { useSessionChangesCount } from "./use-session-changes-count";

type FileEntry = { path: string; status: "modified"; staged: boolean };
type StatusEntry = {
  branch: string;
  files: Record<string, FileEntry>;
  timestamp: string;
  repository_name?: string;
};

function setStore(opts: {
  envBySession?: Record<string, string>;
  byEnvironmentRepo?: Record<string, Record<string, StatusEntry>>;
  commitsByEnvironmentId?: Record<string, unknown[]>;
}) {
  storeState = {
    environmentIdBySessionId: opts.envBySession ?? {},
    gitStatus: {
      byEnvironmentId: {} as Record<string, StatusEntry>,
      byEnvironmentRepo: opts.byEnvironmentRepo ?? {},
    },
    sessionCommits: {
      byEnvironmentId: opts.commitsByEnvironmentId ?? {},
      loading: {} as Record<string, boolean>,
    },
    connection: { status: "disconnected" },
    setSessionCommits: vi.fn(),
    setSessionCommitsLoading: vi.fn(),
  };
}

function file(path: string): FileEntry {
  return { path, status: "modified", staged: false };
}

function status(files: string[], repository_name?: string): StatusEntry {
  const map: Record<string, FileEntry> = {};
  for (const p of files) map[p] = file(p);
  return {
    branch: "feature",
    files: map,
    timestamp: "t",
    repository_name,
  };
}

describe("useSessionChangesCount", () => {
  beforeEach(() => {
    setStore({});
  });

  afterEach(() => {
    cleanup();
  });

  it("returns 0 when the session has no gitStatus and no commits yet", () => {
    const { result } = renderHook(() => useSessionChangesCount("sess-new"));
    expect(result.current).toBe(0);
  });

  it("returns 0 for null session id", () => {
    const { result } = renderHook(() => useSessionChangesCount(null));
    expect(result.current).toBe(0);
  });

  it("counts files from a single-repo workspace via the empty repo key", () => {
    setStore({
      envBySession: { "sess-1": "env-1" },
      byEnvironmentRepo: { "env-1": { "": status(["a.ts", "b.ts"]) } },
    });
    const { result } = renderHook(() => useSessionChangesCount("sess-1"));
    expect(result.current).toBe(2);
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
    const { result } = renderHook(() => useSessionChangesCount("sess-1"));
    expect(result.current).toBe(3);
  });

  it("includes commits in the total count", () => {
    setStore({
      envBySession: { "sess-1": "env-1" },
      byEnvironmentRepo: { "env-1": { "": status(["a.ts"]) } },
      commitsByEnvironmentId: { "env-1": [{ commit_sha: "x" }, { commit_sha: "y" }] },
    });
    const { result } = renderHook(() => useSessionChangesCount("sess-1"));
    expect(result.current).toBe(3);
  });

  it("falls back to sessionId when no environment mapping is registered yet", () => {
    setStore({
      byEnvironmentRepo: { "sess-pending": { "": status(["only.ts"]) } },
    });
    const { result } = renderHook(() => useSessionChangesCount("sess-pending"));
    expect(result.current).toBe(1);
  });

  it("does not leak stale data from a different session's environment", () => {
    // The bug: a brand-new task whose env has no gitStatus yet must not pick
    // up another task's count just because their env IDs happen to share the
    // map. The selector keys strictly by envKey resolved from sessionId.
    setStore({
      envBySession: { "sess-old": "env-old", "sess-new": "env-new" },
      byEnvironmentRepo: { "env-old": { "": status(["leak.ts", "leak2.ts"]) } },
      commitsByEnvironmentId: { "env-old": [{ commit_sha: "old" }] },
    });
    const { result } = renderHook(() => useSessionChangesCount("sess-new"));
    expect(result.current).toBe(0);
  });
});
