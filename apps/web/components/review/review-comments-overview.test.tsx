import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup, within } from "@testing-library/react";
import type { DiffComment } from "@/lib/diff/types";
import { ReviewCommentsOverview, groupCommentsByFile } from "./review-comments-overview";

afterEach(cleanup);

const APP_PATH = "src/app.tsx";

function comment(overrides: Partial<DiffComment>): DiffComment {
  return {
    id: "c1",
    sessionId: "s1",
    source: "diff",
    filePath: APP_PATH,
    startLine: 10,
    endLine: 10,
    side: "additions",
    codeContent: "const x = 1;",
    text: "fix this",
    createdAt: "2026-01-01T00:00:00Z",
    status: "pending",
    ...overrides,
  };
}

describe("groupCommentsByFile", () => {
  it("groups comments by file preserving first-seen order", () => {
    const groups = groupCommentsByFile([
      comment({ id: "a", filePath: "b.ts" }),
      comment({ id: "b", filePath: "a.ts" }),
      comment({ id: "c", filePath: "b.ts" }),
    ]);
    expect(groups.map((g) => g.filePath)).toEqual(["b.ts", "a.ts"]);
    expect(groups[0].comments.map((c) => c.id)).toEqual(["a", "c"]);
    expect(groups[1].comments.map((c) => c.id)).toEqual(["b"]);
  });

  it("returns an empty list for no comments", () => {
    expect(groupCommentsByFile([])).toEqual([]);
  });
});

describe("ReviewCommentsOverview", () => {
  it("shows an empty state when there are no comments", () => {
    render(<ReviewCommentsOverview comments={[]} />);
    expect(screen.getByText(/No comments to fix yet/i)).toBeDefined();
    expect(screen.queryByTestId("review-comments-overview")).toBeNull();
  });

  it("summarizes comment and file totals with pluralization", () => {
    render(
      <ReviewCommentsOverview
        comments={[
          comment({ id: "a", filePath: APP_PATH }),
          comment({ id: "b", filePath: "src/util.ts" }),
          comment({ id: "c", filePath: "src/util.ts" }),
        ]}
      />,
    );
    expect(screen.getByText("3 pending review comments")).toBeDefined();
    expect(screen.getByText("across 2 files")).toBeDefined();
  });

  it("uses singular labels for a single comment on a single file", () => {
    render(<ReviewCommentsOverview comments={[comment({ id: "a" })]} />);
    expect(screen.getByText("1 pending review comment")).toBeDefined();
    expect(screen.getByText("across 1 file")).toBeDefined();
  });

  it("renders the file name, line range, side and text for each comment", () => {
    render(
      <ReviewCommentsOverview
        comments={[
          comment({ id: "a", filePath: APP_PATH, startLine: 5, endLine: 8, text: "range note" }),
          comment({ id: "b", filePath: APP_PATH, side: "deletions", text: "removed note" }),
        ]}
      />,
    );
    expect(screen.getByText("app.tsx")).toBeDefined();
    expect(screen.getByText("src")).toBeDefined();
    expect(screen.getByText("L5-8")).toBeDefined();
    expect(screen.getByText(/new/)).toBeDefined();
    expect(screen.getByText(/old/)).toBeDefined();
    expect(screen.getByText("range note")).toBeDefined();
    expect(screen.getByText("removed note")).toBeDefined();
  });

  it("shows a per-file comment count badge", () => {
    render(
      <ReviewCommentsOverview
        comments={[
          comment({ id: "a", filePath: APP_PATH }),
          comment({ id: "b", filePath: APP_PATH }),
        ]}
      />,
    );
    const overview = screen.getByTestId("review-comments-overview");
    expect(within(overview).getByText("2")).toBeDefined();
  });
});
