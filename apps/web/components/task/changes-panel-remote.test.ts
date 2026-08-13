import { describe, expect, it } from "vitest";
import { mergeCommits, separateCommitHistories } from "./changes-panel";

const PR_DATE = "2026-03-29T00:00:00Z";
const SHARED_SHA = "ccc3333";
const WIDGET_REPO = "widget";
const WORKSPACE_ID = "workspace-1";

function makeLocal(sha: string, message = "msg") {
  return { commit_sha: sha, commit_message: message, insertions: 1, deletions: 0 };
}

function makePR(sha: string, message = "msg") {
  return {
    sha,
    message,
    author_login: "user",
    author_date: PR_DATE,
    additions: 5,
    deletions: 2,
    files_changed: 1,
  };
}

describe("mergeCommits source provenance", () => {
  it("places provider-only commits newest first before shared history", () => {
    const local = [makeLocal(SHARED_SHA, "shared")];
    const provider = [
      {
        ...makePR("old1111", "old provider"),
        stats_available: false,
        workspace_id: WORKSPACE_ID,
        owner: "acme",
        repo: WIDGET_REPO,
      },
      {
        ...makePR(SHARED_SHA, "shared provider"),
        stats_available: false,
        workspace_id: WORKSPACE_ID,
        owner: "acme",
        repo: WIDGET_REPO,
      },
      {
        ...makePR("new9999", "new provider"),
        stats_available: false,
        workspace_id: WORKSPACE_ID,
        owner: "acme",
        repo: WIDGET_REPO,
      },
    ];

    const result = mergeCommits(local, provider);

    expect(result.map((commit) => commit.commit_sha)).toEqual(["new9999", SHARED_SHA, "old1111"]);
    expect(result[0]).toMatchObject({ presentation: "current_pr", pushed: true });
    expect(result[1]).toMatchObject({ commit_message: "shared" });
    expect(result[1].presentation).toBeUndefined();
    expect(result[2]).toMatchObject({ presentation: "current_pr", pushed: true });
  });

  it("marks PR-only commits as remote with explicitly unavailable stats", () => {
    const pr = {
      ...makePR(SHARED_SHA, "external fix"),
      stats_available: false,
      workspace_id: WORKSPACE_ID,
      owner: "acme",
      repo: WIDGET_REPO,
      repository_name: WIDGET_REPO,
    };

    expect(mergeCommits([], [pr])).toMatchObject([
      {
        statsAvailable: false,
        detailTarget: {
          source: "github",
          sha: SHARED_SHA,
          workspaceId: WORKSPACE_ID,
          owner: "acme",
          repo: WIDGET_REPO,
        },
      },
    ]);
  });

  it("prefers the local source when a SHA exists in both feeds", () => {
    const local = [{ ...makeLocal(SHARED_SHA, "local"), repository_name: WIDGET_REPO }];
    const pr = {
      ...makePR(SHARED_SHA, "remote"),
      stats_available: false,
      workspace_id: WORKSPACE_ID,
      owner: "acme",
      repo: WIDGET_REPO,
      repository_name: WIDGET_REPO,
    };

    expect(mergeCommits(local, [pr])).toMatchObject([
      {
        commit_message: "local",
        statsAvailable: true,
        detailTarget: { source: "local", sha: SHARED_SHA, repo: WIDGET_REPO },
      },
    ]);
  });
});

describe("mergeCommits repository identity", () => {
  it("does not deduplicate equal SHAs from different repositories", () => {
    const local = [{ ...makeLocal(SHARED_SHA, "local"), repository_name: "widget-a" }];
    const pr = {
      ...makePR(SHARED_SHA, "remote"),
      stats_available: false,
      workspace_id: WORKSPACE_ID,
      owner: "acme",
      repo: "widget-b",
      repository_name: "widget-b",
    };

    const result = mergeCommits(local, [pr]);
    expect(result).toHaveLength(2);
    expect(result[1]).toMatchObject({
      repository_name: "widget-b",
      detailTarget: { source: "github", repo: "widget-b" },
    });
  });

  it("keeps a same-SHA PR-only commit when another repository matches locally", () => {
    const local = [{ ...makeLocal(SHARED_SHA, "local"), repository_name: "widget-a" }];
    const matchedPR = {
      ...makePR(SHARED_SHA, "local PR"),
      stats_available: false,
      workspace_id: WORKSPACE_ID,
      owner: "acme",
      repo: "widget-a",
      repository_name: "widget-a",
    };
    const remotePR = {
      ...makePR(SHARED_SHA, "remote PR"),
      stats_available: false,
      workspace_id: WORKSPACE_ID,
      owner: "acme",
      repo: "widget-b",
      repository_name: "widget-b",
    };

    const result = mergeCommits(local, [matchedPR, remotePR]);

    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({
      commit_message: "local",
      detailTarget: { source: "local", repo: "widget-a" },
    });
    expect(result[1]).toMatchObject({
      commit_message: "remote PR",
      repository_name: "widget-b",
      detailTarget: { source: "github", repo: "widget-b" },
    });
  });
});

describe("separateCommitHistories", () => {
  it("preserves rewritten provider and checkout rows without deduplication", () => {
    const local = [makeLocal("old1111", "same message"), makeLocal("old2222", "local work")];
    const provider = [
      {
        ...makePR("new1111", "same message"),
        stats_available: false,
        workspace_id: WORKSPACE_ID,
        owner: "acme",
        repo: WIDGET_REPO,
      },
      {
        ...makePR("new3333", "provider work"),
        stats_available: false,
        workspace_id: WORKSPACE_ID,
        owner: "acme",
        repo: WIDGET_REPO,
      },
    ];

    const result = separateCommitHistories(local, provider);

    expect(result.providerCommits.map((commit) => commit.commit_sha)).toEqual([
      "new1111",
      "new3333",
    ]);
    expect(result.localCommits.map((commit) => commit.commit_sha)).toEqual(["old1111", "old2222"]);
    expect(result.providerCommits.every((commit) => commit.presentation === "current_pr")).toBe(
      true,
    );
    expect(result.localCommits.every((commit) => commit.presentation === "local_checkout")).toBe(
      true,
    );
    expect(result.localCommits.every((commit) => commit.pushed === false)).toBe(true);
  });
  it("does not create a PR-only target without complete provenance", () => {
    const pr = {
      ...makePR(SHARED_SHA, "remote"),
      stats_available: false,
      workspace_id: "",
      owner: "acme",
      repo: WIDGET_REPO,
      repository_name: WIDGET_REPO,
    };

    expect(mergeCommits([], [pr])).toEqual([]);
  });

  it("does not match a root local commit to a named repository PR", () => {
    const local = [makeLocal(SHARED_SHA, "root local")];
    const pr = {
      ...makePR(SHARED_SHA, "remote"),
      stats_available: false,
      workspace_id: WORKSPACE_ID,
      owner: "acme",
      repo: WIDGET_REPO,
      repository_name: WIDGET_REPO,
    };

    const result = mergeCommits(local, [pr]);

    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({
      commit_message: "root local",
      pushed: false,
      detailTarget: { source: "local", sha: SHARED_SHA },
    });
    expect(result[1]).toMatchObject({
      commit_message: "remote",
      pushed: true,
      repository_name: WIDGET_REPO,
      detailTarget: { source: "github", repo: WIDGET_REPO },
    });
  });
});
