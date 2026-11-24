import { expect, it } from "vitest";
import { commentAttachmentsForDelivery, formatReviewCommentsAsMarkdown } from "./format";
import type { ReviewComment } from "./types";

it("keeps equal filenames attached below their respective review comments", () => {
  const comments: ReviewComment[] = ["first", "second"].map((id) => ({
    id: `src/main.go-${id}`,
    source: "review-file",
    status: "pending",
    sessionId: "session",
    createdAt: "",
    filePath: "example.go",
    repositoryName: "repo",
    text: id,
    attachments: [
      {
        attachment_id: `upload-${id}`,
        name: "notes.txt",
        type: "resource",
        mime_type: "text/plain",
        delivery_mode: "path",
      },
    ],
  }));
  expect(formatReviewCommentsAsMarkdown(comments)).toContain(
    "> first\n\n<!-- kandev-comment-files:upload-first -->",
  );
  expect(
    commentAttachmentsForDelivery(comments).map((file) => [file.comment_id, file.attachment_id]),
  ).toEqual([
    ["upload-first", "upload-first"],
    ["upload-second", "upload-second"],
  ]);
});
