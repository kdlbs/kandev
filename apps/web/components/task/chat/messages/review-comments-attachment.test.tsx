import { afterEach, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ReviewCommentsAttachment } from "./review-comments-attachment";
import type { ReviewFileComment } from "@/lib/state/slices/comments";

afterEach(cleanup);
it("shows separate repository context and whole-file labels without line metadata", () => {
  const comments = ["api", "web"].map(
    (repositoryName): ReviewFileComment => ({
      source: "review-file",
      id: repositoryName,
      repositoryName,
      repositoryId: repositoryName,
      sessionId: "s",
      filePath: "README.md",
      text: `Feedback for ${repositoryName}`,
      status: "pending",
      createdAt: "now",
    }),
  );
  render(<ReviewCommentsAttachment comments={comments} />);
  fireEvent.click(screen.getByRole("button"));
  expect(screen.getByText("api/README.md")).toBeDefined();
  expect(screen.getByText("web/README.md")).toBeDefined();
  expect(screen.getAllByText("File comment")).toHaveLength(2);
  expect(document.body.textContent).not.toContain("undefined");
});
