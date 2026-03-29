import { describe, it, expect } from "vitest";
import { filterUnpushedCommits } from "./changes-panel";

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
