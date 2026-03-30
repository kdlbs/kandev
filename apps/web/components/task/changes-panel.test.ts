import { describe, it, expect } from "vitest";
import { filterUnpushedCommits, mergeCommits } from "./changes-panel";

const makeLocal = (sha: string, message = "msg") => ({
  commit_sha: sha,
  commit_message: message,
  insertions: 1,
  deletions: 0,
});

const makePR = (sha: string) => ({
  sha,
  message: "msg",
  author_login: "user",
  author_date: "2026-03-29T00:00:00Z",
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
  author_date: "2026-03-29T00:00:00Z",
  additions: 5,
  deletions: 2,
  files_changed: 1,
});

describe("mergeCommits", () => {
  it("marks all local commits as unpushed when no PR commits exist", () => {
    const local = [makeLocal("aaa1111", "first"), makeLocal("bbb2222", "second")];
    const result = mergeCommits(local, []);
    expect(result).toEqual([
      {
        commit_sha: "aaa1111",
        commit_message: "first",
        insertions: 1,
        deletions: 0,
        pushed: false,
      },
      {
        commit_sha: "bbb2222",
        commit_message: "second",
        insertions: 1,
        deletions: 0,
        pushed: false,
      },
    ]);
  });

  it("marks local commits as pushed when they match PR commits", () => {
    const local = [makeLocal("aaa1111", "first")];
    const pr = [makePRFull("aaa1111bbbccc", "first", "user")];
    const result = mergeCommits(local, pr);
    expect(result).toEqual([
      { commit_sha: "aaa1111", commit_message: "first", insertions: 1, deletions: 0, pushed: true },
    ]);
  });

  it("includes PR-only commits (from other contributors) as pushed", () => {
    const local: ReturnType<typeof makeLocal>[] = [];
    const pr = [makePRFull("ccc3333", "external fix", "other-dev")];
    const result = mergeCommits(local, pr);
    expect(result).toEqual([
      {
        commit_sha: "ccc3333",
        commit_message: "external fix",
        insertions: 5,
        deletions: 2,
        pushed: true,
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
});
