import { expect, it } from "vitest";
import { groupCommentsByFile } from "./group-review";
import type { DiffComment, ReviewFileComment } from "./types";

function whole(repositoryName: string): ReviewFileComment {
  return {
    id: repositoryName,
    source: "review-file",
    repositoryName,
    repositoryId: "shared",
    filePath: "README.md",
    sessionId: "session",
    text: "Feedback",
    status: "pending",
    createdAt: "now",
  };
}
function line(): DiffComment {
  return {
    id: "line",
    source: "diff",
    repositoryId: "shared",
    filePath: "README.md",
    sessionId: "session",
    text: "Line feedback",
    status: "pending",
    createdAt: "now",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "content",
  };
}
it("keeps nested scopes separate even when they share a repository ID", () => {
  const groups = groupCommentsByFile([whole("nested"), whole("other")]);
  expect(groups.map((g) => g.filePath)).toEqual(["nested/README.md", "other/README.md"]);
  expect(groups.map((g) => g.comments.length)).toEqual([1, 1]);
});
it("does not assign legacy line feedback to an ambiguous nested scope", () => {
  const groups = groupCommentsByFile([line(), whole("nested"), whole("other")]);
  expect(groups.map((g) => g.comments.map((c) => c.id))).toEqual([["line"], ["nested"], ["other"]]);
});
it("bridges line feedback by ID when the repository name is unambiguous", () => {
  const groups = groupCommentsByFile([line(), whole("nested")]);
  expect(groups).toHaveLength(1);
  expect(groups[0].filePath).toBe("nested/README.md");
  expect(groups[0].comments.map((c) => c.id)).toEqual(["line", "nested"]);
});
it("keeps whole-file identity stable when repository ID resolution changes", () => {
  const resolved = whole("nested");
  const unresolved = { ...resolved, id: "unresolved", repositoryId: undefined };
  expect(groupCommentsByFile([resolved, unresolved])).toHaveLength(1);
});
