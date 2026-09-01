import { describe, expect, it } from "vitest";
import type { PRComment, PRFeedback } from "@/lib/types/github";
import { mapGitHubComments } from "./pr-detail-panel";

const REVIEW_TIMESTAMP = "2026-01-05T12:00:00Z";
const ISSUE_TIMESTAMP = "2026-01-05T09:00:00Z";
const REVIEW_COMMENT_URL = "https://github.com/acme/widget/pull/42#discussion_r20";
const CONVERSATION_COMMENT_URL = "https://github.com/acme/widget/pull/42#issuecomment-10";

describe("mapGitHubComments", () => {
  it("preserves the exact GitHub permalink for every comment type", () => {
    const comments: PRComment[] = [
      {
        id: 20,
        html_url: REVIEW_COMMENT_URL,
        author: "alice",
        author_avatar: "",
        author_is_bot: false,
        body: "Inline note",
        path: "main.go",
        line: 7,
        side: "RIGHT",
        comment_type: "review",
        created_at: REVIEW_TIMESTAMP,
        updated_at: REVIEW_TIMESTAMP,
        in_reply_to: null,
      },
      {
        id: 10,
        html_url: CONVERSATION_COMMENT_URL,
        author: "dependabot",
        author_avatar: "",
        author_is_bot: true,
        body: "Conversation note",
        path: "",
        line: 0,
        side: "",
        comment_type: "issue",
        created_at: ISSUE_TIMESTAMP,
        updated_at: ISSUE_TIMESTAMP,
        in_reply_to: null,
      },
    ];

    expect(mapGitHubComments({ comments } as unknown as PRFeedback)).toMatchObject([
      {
        id: "20",
        url: REVIEW_COMMENT_URL,
      },
      {
        id: "10",
        url: CONVERSATION_COMMENT_URL,
      },
    ]);
  });

  it("does not invent a URL when GitHub omits one", () => {
    const comment = {
      id: 20,
      html_url: "",
      author: "alice",
      author_avatar: "",
      author_is_bot: false,
      body: "Inline note",
      path: "main.go",
      line: 7,
      side: "RIGHT",
      comment_type: "review",
      created_at: REVIEW_TIMESTAMP,
      updated_at: REVIEW_TIMESTAMP,
      in_reply_to: null,
    } satisfies PRComment;

    expect(
      mapGitHubComments({ comments: [comment] } as unknown as PRFeedback)[0],
    ).not.toHaveProperty("url");
  });

  it("normalizes whitespace around a provider URL", () => {
    const comment = {
      id: 20,
      html_url: `  ${REVIEW_COMMENT_URL}  `,
      author: "alice",
      author_avatar: "",
      author_is_bot: false,
      body: "Inline note",
      path: "main.go",
      line: 7,
      side: "RIGHT",
      comment_type: "review",
      created_at: REVIEW_TIMESTAMP,
      updated_at: REVIEW_TIMESTAMP,
      in_reply_to: null,
    } satisfies PRComment;

    expect(mapGitHubComments({ comments: [comment] } as unknown as PRFeedback)[0]).toMatchObject({
      url: REVIEW_COMMENT_URL,
    });
  });
});
