import { describe, expect, it } from "vitest";
import { resolveSessionWorktrees } from "./use-session-worktrees";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { RepositoryId, SessionId, TaskSession, TaskSessionWorktree } from "@/lib/types/http";

const SESSION_ID = "session-1" as SessionId;

function session(overrides: Partial<TaskSession>): TaskSession {
  return { id: SESSION_ID, task_id: "task-1", state: "RUNNING", ...overrides } as TaskSession;
}

function apiWorktree(overrides: Partial<TaskSessionWorktree>): TaskSessionWorktree {
  return {
    id: "assoc-1",
    session_id: SESSION_ID,
    worktree_id: "wt-1",
    position: 0,
    ...overrides,
  };
}

describe("resolveSessionWorktrees", () => {
  it("prefers the API worktree list, ordered by position", () => {
    const result = resolveSessionWorktrees(
      SESSION_ID,
      session({
        worktrees: [
          apiWorktree({
            id: "assoc-2",
            worktree_id: "wt-2",
            position: 1,
            repository_id: "repo-b" as RepositoryId,
            worktree_path: "/x/b",
            worktree_branch: "kd/b",
          }),
          apiWorktree({
            id: "assoc-1",
            worktree_id: "wt-1",
            position: 0,
            repository_id: "repo-a" as RepositoryId,
            worktree_path: "/x/a",
            worktree_branch: "kd/a",
          }),
        ],
      }),
      {},
      undefined,
    );
    expect(result).toEqual([
      { id: "wt-1", sessionId: SESSION_ID, repositoryId: "repo-a", path: "/x/a", branch: "kd/a" },
      { id: "wt-2", sessionId: SESSION_ID, repositoryId: "repo-b", path: "/x/b", branch: "kd/b" },
    ]);
  });

  it("fills in missing API fields from the live worktrees map", () => {
    const live: Record<string, Worktree> = {
      "wt-1": { id: "wt-1", sessionId: SESSION_ID, repositoryId: "repo-a", path: "/x/a" },
    };
    const result = resolveSessionWorktrees(
      SESSION_ID,
      session({ worktrees: [apiWorktree({ worktree_id: "wt-1" })] }),
      live,
      undefined,
    );
    expect(result[0]).toMatchObject({ id: "wt-1", repositoryId: "repo-a", path: "/x/a" });
  });

  it("treats empty-string API fields as missing and falls back to live values", () => {
    const live: Record<string, Worktree> = {
      "wt-1": { id: "wt-1", sessionId: SESSION_ID, path: "/x/a", branch: "kd/a" },
    };
    const result = resolveSessionWorktrees(
      SESSION_ID,
      session({
        worktrees: [apiWorktree({ worktree_id: "wt-1", worktree_path: "", worktree_branch: "" })],
      }),
      live,
      undefined,
    );
    expect(result[0]).toMatchObject({ id: "wt-1", path: "/x/a", branch: "kd/a" });
  });

  it("unions WS-only worktrees with the API list", () => {
    const live: Record<string, Worktree> = {
      "wt-1": { id: "wt-1", sessionId: SESSION_ID, path: "/x/a" },
      "wt-2": { id: "wt-2", sessionId: SESSION_ID, path: "/x/b" },
    };
    const result = resolveSessionWorktrees(
      SESSION_ID,
      session({ worktrees: [apiWorktree({ worktree_id: "wt-1", worktree_path: "/x/a" })] }),
      live,
      ["wt-1", "wt-2"],
    );
    expect(result.map((wt) => wt.id)).toEqual(["wt-1", "wt-2"]);
  });

  it("falls back to the WS per-session list when the API list is empty", () => {
    const live: Record<string, Worktree> = {
      "wt-1": { id: "wt-1", sessionId: SESSION_ID, path: "/x/a" },
    };
    const result = resolveSessionWorktrees(SESSION_ID, session({}), live, ["wt-1", "wt-gone"]);
    expect(result).toEqual([live["wt-1"]]);
  });

  it("falls back to the legacy single worktree_id field", () => {
    const live: Record<string, Worktree> = {
      "wt-1": { id: "wt-1", sessionId: SESSION_ID, path: "/x/a" },
    };
    const result = resolveSessionWorktrees(
      SESSION_ID,
      session({ worktree_id: "wt-1" }),
      live,
      undefined,
    );
    expect(result).toEqual([live["wt-1"]]);
  });

  it("returns empty when nothing is known", () => {
    expect(resolveSessionWorktrees(SESSION_ID, session({}), {}, undefined)).toEqual([]);
  });
});
