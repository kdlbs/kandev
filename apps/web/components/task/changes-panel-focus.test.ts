import { describe, expect, it } from "vitest";
import {
  markInactiveChangesCountIncreases,
  selectChangesCountByEnvironment,
} from "./changes-panel-focus";
import type { GitStatusEntry, SessionCommit } from "@/lib/state/slices/session-runtime/types";

function gitStatus(files: string[]): GitStatusEntry {
  return {
    branch: "main",
    remote_branch: null,
    modified: [],
    added: [],
    deleted: [],
    untracked: files,
    renamed: [],
    ahead: 0,
    behind: 0,
    files: Object.fromEntries(
      files.map((path) => [path, { path, status: "untracked", staged: false }]),
    ),
    timestamp: "2026-06-29T00:00:00Z",
  };
}

function commit(sha: string): SessionCommit {
  return {
    id: `commit-${sha}`,
    session_id: "sess-1",
    commit_sha: sha,
    parent_sha: `parent-${sha}`,
    author_name: "Test User",
    author_email: "test@example.com",
    commit_message: `Commit ${sha}`,
    committed_at: "2026-06-29T00:00:00Z",
    files_changed: 1,
    insertions: 1,
    deletions: 0,
    created_at: "2026-06-29T00:00:00Z",
  };
}

describe("selectChangesCountByEnvironment", () => {
  it("counts git files and commits per environment", () => {
    const state = {
      gitStatus: {
        byEnvironmentId: {},
        byEnvironmentRepo: {
          envA: {
            repo1: gitStatus(["one.ts", "two.ts"]),
          },
          envB: {
            repo1: gitStatus([]),
            repo2: gitStatus(["three.ts"]),
          },
        },
      },
      sessionCommits: {
        loading: {},
        refetchTrigger: {},
        byEnvironmentId: {
          envA: [commit("commit-1"), commit("commit-2")],
          envC: [commit("commit-3")],
        },
      },
    };

    expect(selectChangesCountByEnvironment(state)).toEqual({
      envA: 4,
      envB: 1,
      envC: 1,
    });
  });
});

describe("markInactiveChangesCountIncreases", () => {
  it("baselines first observations and queues only inactive environment increases", () => {
    const previousCounts: Record<string, number> = {};
    const pendingEnvKeys = new Set<string>();

    markInactiveChangesCountIncreases({
      countsByEnv: { envA: 1, envB: 0 },
      activeEnvKey: "envA",
      previousCounts,
      pendingEnvKeys,
    });

    expect(pendingEnvKeys.size).toBe(0);

    markInactiveChangesCountIncreases({
      countsByEnv: { envA: 2, envB: 1 },
      activeEnvKey: "envA",
      previousCounts,
      pendingEnvKeys,
    });

    expect([...pendingEnvKeys]).toEqual(["envB"]);
    expect(previousCounts).toEqual({ envA: 2, envB: 1 });
  });
});
