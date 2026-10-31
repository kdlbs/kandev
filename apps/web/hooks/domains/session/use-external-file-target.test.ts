import { describe, expect, it } from "vitest";
import { repositoryId, sessionId, taskId, type TaskSession } from "@/lib/types/http";
import type { Worktree } from "@/lib/state/slices/session/types";
import { resolveExternalFileTarget } from "./use-external-file-target";

const session = {
  id: sessionId("s"),
  task_id: taskId("t"),
  state: "WAITING_FOR_INPUT",
  repository_id: repositoryId("repo-a"),
  worktree_id: "wt-a",
  worktree_path: "/task/a",
  workspace_path: "/task",
} as TaskSession;
const a: Worktree = { id: "wt-a", sessionId: "s", repositoryId: "repo-a", path: "/task/a" };
const b: Worktree = { id: "wt-b", sessionId: "s", repositoryId: "repo-b", path: "/task/b" };

describe("external file target", () => {
  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.1, AC-UI-EXTERNAL-FILE-TARGET-001.3
  it("uses a unique file-specific ID without placement or a name", () => {
    const legacy = { ...session, workspace_path: undefined };
    expect(
      resolveExternalFileTarget(legacy, [a, b], { repositoryId: "repo-b", repositoryName: "b" }),
    ).toEqual({ worktreeId: "wt-b" });
    expect(resolveExternalFileTarget(legacy, [a, b], { repositoryId: "repo-b" })).toEqual({
      worktreeId: "wt-b",
    });
    expect(
      resolveExternalFileTarget(session, [a, b], {
        repositoryId: "repo-b",
        repositoryName: "unknown",
      }),
    ).toEqual({ worktreeId: "wt-b" });
  });
  it("rejects a proven conflict but not ambiguous redundant names", () => {
    expect(
      resolveExternalFileTarget(session, [a, b], { repositoryId: "repo-b", repositoryName: "a" }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget(session, [a, b, { ...a, id: "wt-c" }], {
        repositoryId: "repo-b",
        repositoryName: "a",
      }),
    ).toEqual({ worktreeId: "wt-b" });
  });
  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.2, AC-UI-EXTERNAL-FILE-TARGET-001.3
  it("matches name-only scopes by observed workspace placement", () => {
    expect(resolveExternalFileTarget(session, [a, b], { repositoryName: "b" })).toEqual({
      worktreeId: "wt-b",
    });
    expect(
      resolveExternalFileTarget({ ...session, workspace_path: undefined }, [a, b], {
        repositoryName: "b",
      }),
    ).toBeNull();
    expect(resolveExternalFileTarget(session, [a, b], { repositoryName: "unknown" })).toBeNull();
    expect(resolveExternalFileTarget(session, [a, b], { repositoryName: "b/nested" })).toBeNull();
    expect(
      resolveExternalFileTarget(session, [a, { ...a, id: "duplicate" }], { repositoryName: "a" }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget(session, [a, { ...a, id: "duplicate" }], {
        repositoryId: "repo-a",
      }),
    ).toBeNull();
    expect(resolveExternalFileTarget(null, [a], { repositoryId: "repo-a" })).toBeNull();
  });
  // @covers AC-UI-EXTERNAL-FILE-TARGET-001.4
  it("keeps only supported legacy and single-worktree defaults", () => {
    expect(resolveExternalFileTarget(session, [a], {})).toEqual({ worktreeId: "wt-a" });
    expect(resolveExternalFileTarget(session, [a, b], {})).toBeNull();
    expect(resolveExternalFileTarget(session, [], {})).toEqual({});
    expect(resolveExternalFileTarget(session, [], { repositoryId: "repo-a" })).toEqual({});
    expect(resolveExternalFileTarget(session, [], { repositoryId: "repo-b" })).toBeNull();
    expect(resolveExternalFileTarget(session, [], { repositoryName: "a" })).toBeNull();
    expect(
      resolveExternalFileTarget(
        { ...session, worktree_id: undefined, worktree_path: undefined, repository_id: undefined },
        [],
        {},
      ),
    ).toBeNull();
  });
});

describe("legacy single worktree", () => {
  it("preserves the default only with matching primary evidence", () => {
    const legacy = { ...session, worktrees: undefined };
    const anonymous: Worktree = { id: "wt-a", sessionId: "s", path: "/task/a" };
    expect(resolveExternalFileTarget(legacy, [anonymous], { repositoryId: "repo-a" })).toEqual({});
    expect(
      resolveExternalFileTarget({ ...legacy, worktrees: [] }, [anonymous], {
        repositoryId: "repo-a",
      }),
    ).toEqual({});
    expect(resolveExternalFileTarget(legacy, [anonymous], { repositoryId: "repo-b" })).toBeNull();
    expect(
      resolveExternalFileTarget(legacy, [anonymous], {
        repositoryId: "repo-a",
        repositoryName: "unknown",
      }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget(legacy, [{ ...anonymous, id: "wt-other" }], {
        repositoryId: "repo-a",
      }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget(legacy, [{ ...anonymous, repositoryId: "repo-b" }], {
        repositoryId: "repo-a",
      }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget(legacy, [anonymous, { ...anonymous, id: "wt-other" }], {
        repositoryId: "repo-a",
      }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget({ ...legacy, worktree_id: undefined }, [anonymous], {
        repositoryId: "repo-a",
      }),
    ).toBeNull();
    expect(
      resolveExternalFileTarget(
        {
          ...legacy,
          worktrees: [{ id: "assoc", session_id: legacy.id, worktree_id: "wt-a", position: 0 }],
        },
        [anonymous],
        { repositoryId: "repo-a" },
      ),
    ).toBeNull();
  });
});
