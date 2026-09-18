import { expect, it } from "vitest";
import { computeCommentCounts, filterPendingReviewCommentsForSession } from "./review-dialog";
import type { ReviewFileComment } from "@/lib/state/slices/comments";
import { reviewFileKey, type ReviewFile } from "./types";

it("counts whole-file feedback only in its repository scope, including shared IDs", () => {
  const comment: ReviewFileComment = {
    id: "c",
    sessionId: "s",
    source: "review-file",
    filePath: "a.txt",
    repositoryName: "nested",
    repositoryId: "same",
    text: "note",
    status: "pending",
    createdAt: "now",
  };
  const files = ["", "nested", "other"].map(
    (repository_name): ReviewFile => ({
      path: "a.txt",
      repository_name,
      repository_id: "same",
      status: "modified",
      staged: false,
      source: "uncommitted",
      diff: "",
      additions: 0,
      deletions: 0,
    }),
  );
  expect(
    computeCommentCounts(
      { c: comment },
      ["c"],
      files,
      new Map([
        ["nested", "same"],
        ["other", "same"],
      ]),
    ),
  ).toEqual({ [reviewFileKey(files[1])]: 1 });
  expect(filterPendingReviewCommentsForSession([comment], "other")).toEqual([]);
});

it.each([undefined, ""])("counts root file comments for repository name %s", (repository_name) => {
  const file: ReviewFile = {
    path: "root.txt",
    repository_name,
    status: "modified",
    source: "uncommitted",
    staged: false,
    diff: "",
    additions: 0,
    deletions: 0,
  };
  const comment: ReviewFileComment = {
    id: "c",
    sessionId: "s",
    source: "review-file",
    filePath: file.path,
    repositoryName: "",
    text: "note",
    status: "pending",
    createdAt: "now",
  };
  expect(computeCommentCounts({ c: comment }, ["c"], [file], new Map())).toEqual({
    [reviewFileKey(file)]: 1,
  });
});
