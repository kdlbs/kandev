import { describe, it, expect } from "vitest";
import { filterUnpushedCommits, mergeCommits } from "./changes-panel";

const makeLocal = (sha: string, message = "msg") => ({
  commit_sha: sha,
  commit_message: message,
  insertions: 1,
  deletions: 0,
});

const PR_DATE = "2026-03-29T00:00:00Z";
const WORKSPACE_ID = "workspace-1";
const PR_OWNER = "acme";
const PR_REPO = "widget";
const REMOTE_SHA = "ccc3333";

const makePR = (sha: string) => ({
  sha,
  message: "msg",
  author_login: "user",
  author_date: PR_DATE,
});

describe("filterUnpushedCommits", () => {
  it("returns empty when all local commits are in PR", () => {
    const local = [makeLocal("abc1234def"), makeLocal("fff9999aaa")];
    const pr = [makePR("abc1234def567890"), makePR("fff9999aaa123456")];
    expect(filterUnpushedCommits(local, pr)).toEqual([]);
  });

  it("returns only unpushed commits", () => {
    const local = [makeLocal("abc1234def"), makeLocal("fff9999aaa")];
    const pr = [makePR("abc1234def567890")];
    expect(filterUnpushedCommits(local, pr)).toEqual([makeLocal("fff9999aaa")]);
  });

  it("returns all local commits when no PR commits exist", () => {
    const local = [makeLocal("abc1234def"), makeLocal("fff9999aaa")];
    expect(filterUnpushedCommits(local, [])).toEqual(local);
  });

  it("returns empty when no local commits exist", () => {
    const pr = [makePR("abc1234def567890")];
    expect(filterUnpushedCommits([], pr)).toEqual([]);
  });
});

const makePRFull = (sha: string, message = "msg", author = "user") => ({
  sha,
  message,
  author_login: author,
  author_date: PR_DATE,
  additions: 5,
  deletions: 2,
  files_changed: 1,
  workspace_id: WORKSPACE_ID,
  owner: PR_OWNER,
  repo: PR_REPO,
});

describe("mergeCommits", () => {
  it("marks all local commits as unpushed when no PR commits exist", () => {
    const local = [makeLocal("aaa1111", "first"), makeLocal("bbb2222", "second")];
    const result = mergeCommits(local, []);
    expect(result).toMatchObject([
      {
        commit_sha: "aaa1111",
        pushed: false,
        statsAvailable: true,
        detailTarget: { source: "local", sha: "aaa1111" },
      },
      {
        commit_sha: "bbb2222",
        pushed: false,
        statsAvailable: true,
        detailTarget: { source: "local", sha: "bbb2222" },
      },
    ]);
  });

  it("trusts the backend pushed flag even with no PR (the bug fix)", () => {
    // The original bug: local commits were marked unpushed whenever no PR
    // existed, because pushed was derived from PR-SHA matching alone.
    // Sourcing pushed from git on the backend means commits pushed to a
    // branch without a PR show as pushed.
    const local = [
      { ...makeLocal("aaa1111", "pushed"), pushed: true },
      { ...makeLocal("bbb2222", "local"), pushed: false },
    ];
    const result = mergeCommits(local, []);
    expect(result[0]).toMatchObject({ commit_sha: "bbb2222", pushed: false });
    expect(result[1]).toMatchObject({ commit_sha: "aaa1111", pushed: true });
  });

  it("backend pushed flag wins over PR-SHA mismatch (post-rebase scenario)", () => {
    // After a rebase the local SHA no longer matches the PR's SHA, but the
    // commit IS on the remote (force-pushed). The backend's pushed=true
    // must override the missing PR match.
    const local = [{ ...makeLocal("rebased1", "rebased"), pushed: true }];
    const pr = [makePRFull("oldsha9999", "rebased")];
    const result = mergeCommits(local, pr);
    expect(result[0]).toMatchObject({ commit_sha: "rebased1", pushed: true });
    // PR-only SHA still appears, since it isn't matched.
    expect(result).toHaveLength(2);
    expect(result[1]).toMatchObject({ commit_sha: "oldsha9999", pushed: true });
  });

  it("falls back to PR-SHA matching when backend pushed flag is absent", () => {
    // Older commit_created notifications don't carry the pushed flag.
    // PR matching kicks in as a backstop.
    const local = [makeLocal("aaa1111", "first")];
    const pr = [makePRFull("aaa1111bbbccc", "first", "user")];
    const result = mergeCommits(local, pr);
    expect(result).toMatchObject([
      {
        commit_sha: "aaa1111",
        pushed: true,
        statsAvailable: true,
        detailTarget: { source: "local", sha: "aaa1111" },
      },
    ]);
  });

  it("includes PR-only commits (from other contributors) as pushed", () => {
    const local: ReturnType<typeof makeLocal>[] = [];
    const pr = [makePRFull(REMOTE_SHA, "external fix", "other-dev")];
    const result = mergeCommits(local, pr);
    expect(result).toMatchObject([
      {
        commit_sha: REMOTE_SHA,
        commit_message: "external fix",
        pushed: true,
        committed_at: PR_DATE,
        statsAvailable: false,
        detailTarget: {
          source: "github",
          sha: REMOTE_SHA,
          workspaceId: WORKSPACE_ID,
          owner: PR_OWNER,
          repo: PR_REPO,
        },
      },
    ]);
  });

  it("orders unpushed commits first, then pushed", () => {
    const local = [makeLocal("aaa1111", "pushed one"), makeLocal("bbb2222", "unpushed one")];
    const pr = [makePRFull("aaa1111fff", "pushed one")];
    const result = mergeCommits(local, pr);
    expect(result[0]).toMatchObject({ commit_sha: "bbb2222", pushed: false });
    expect(result[1]).toMatchObject({ commit_sha: "aaa1111", pushed: true });
  });

  it("handles mixed local, matched, and PR-only commits", () => {
    const local = [makeLocal("aaa1111", "local pushed"), makeLocal("bbb2222", "local only")];
    const pr = [makePRFull("aaa1111fff", "local pushed"), makePRFull("ddd4444", "external")];
    const result = mergeCommits(local, pr);
    // unpushed first, then pushed (local matched + PR-only)
    expect(result).toHaveLength(3);
    expect(result[0]).toMatchObject({ commit_sha: "bbb2222", pushed: false });
    expect(result[1]).toMatchObject({ commit_sha: "aaa1111", pushed: true });
    expect(result[2]).toMatchObject({ commit_sha: "ddd4444", pushed: true });
  });

  it("propagates committed_at from local commits", () => {
    const local = [{ ...makeLocal("aaa1111"), committed_at: "2026-05-31T10:00:00Z" }];
    const result = mergeCommits(local, []);
    expect(result[0].committed_at).toBe("2026-05-31T10:00:00Z");
  });

  it("forwards author_date as committed_at for PR-only commits", () => {
    const pr = [makePRFull("ccc3333", "external fix", "other-dev")];
    const result = mergeCommits([], pr);
    expect(result[0].committed_at).toBe(PR_DATE);
  });
});
