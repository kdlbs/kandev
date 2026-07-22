import { describe, expect, it } from "vitest";
import type { GitLabMRDiscussion } from "@/lib/types/gitlab";
import { buildDiscussionContext, buildAllDiscussionsContext } from "./mr-discussions-section";

const discussion: GitLabMRDiscussion = {
  id: "thread-1",
  resolvable: true,
  resolved: false,
  path: "src/server.ts",
  line: 42,
  old_line: 0,
  created_at: "2026-07-20T10:00:00Z",
  updated_at: "2026-07-20T10:01:00Z",
  notes: [
    {
      id: 1,
      author: "alice",
      body: "Handle the nil result.",
      created_at: "2026-07-20T10:00:00Z",
      updated_at: "2026-07-20T10:00:00Z",
    },
    {
      id: 2,
      author: "bob",
      body: "I can reproduce this.",
      created_at: "2026-07-20T10:01:00Z",
      updated_at: "2026-07-20T10:01:00Z",
    },
  ],
};

describe("GitLab MR discussion context", () => {
  it("preserves a threaded discussion, location, and MR terminology", () => {
    const result = buildDiscussionContext(discussion, "https://gitlab.com/a/b/-/merge_requests/7");
    expect(result).toContain("### Merge request discussion");
    expect(result).toContain("`src/server.ts:42`");
    expect(result).toContain("**alice**");
    expect(result).toContain("**bob**");
    expect(result).toContain("Merge request:");
    expect(result).not.toContain("Pull request");
  });

  it("keeps each discussion separate when adding all feedback", () => {
    const second = { ...discussion, id: "thread-2", path: undefined, line: undefined };
    const result = buildAllDiscussionsContext(
      [discussion, second],
      "https://gitlab.com/a/b/-/merge_requests/7",
    );
    expect(result.match(/### Merge request discussion/g)).toHaveLength(2);
  });
});
