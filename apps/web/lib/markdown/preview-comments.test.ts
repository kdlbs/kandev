import { describe, expect, it } from "vitest";
import type { DiffComment } from "@/lib/state/slices/comments";
import {
  buildMarkdownPreviewComment,
  commentsBeginInRange,
  commentsOverlapRange,
} from "./preview-comments";

describe("markdown preview comments", () => {
  it("builds a pending diff comment for selected markdown preview text", () => {
    const comment = buildMarkdownPreviewComment({
      filePath: "README.md",
      repositoryId: "repo-front",
      sessionId: "session-1",
      selectedText: "Rendered paragraph",
      text: "Tighten wording",
      startLine: 3,
      endLine: 5,
    });

    expect(comment).toMatchObject({
      source: "diff",
      sessionId: "session-1",
      repositoryId: "repo-front",
      filePath: "README.md",
      startLine: 3,
      endLine: 5,
      side: "additions",
      codeContent: "Rendered paragraph",
      text: "Tighten wording",
      status: "pending",
    });
    expect(comment.id).toContain("README.md");
  });

  it("detects overlap between a rendered source block and existing comments", () => {
    const comment = {
      source: "diff",
      startLine: 10,
      endLine: 12,
    } as DiffComment;

    expect(commentsOverlapRange([comment], { startLine: 8, endLine: 9 })).toBe(false);
    expect(commentsOverlapRange([comment], { startLine: 9, endLine: 10 })).toBe(true);
    expect(commentsOverlapRange([comment], { startLine: 12, endLine: 14 })).toBe(true);
    expect(commentsOverlapRange([comment], { startLine: 13, endLine: 14 })).toBe(false);
  });

  it("only marks the rendered block where a markdown preview comment begins for the badge", () => {
    const comment = {
      source: "diff",
      startLine: 10,
      endLine: 12,
    } as DiffComment;

    expect(commentsBeginInRange([comment], { startLine: 8, endLine: 9 })).toBe(false);
    expect(commentsBeginInRange([comment], { startLine: 10, endLine: 10 })).toBe(true);
    expect(commentsBeginInRange([comment], { startLine: 11, endLine: 12 })).toBe(false);
    expect(commentsBeginInRange([comment], { startLine: 8, endLine: 14 })).toBe(true);
  });
});
