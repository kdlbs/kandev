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

it("keeps nested repository identities separate when display paths collide", () => {
  const comments: ReviewFileComment[] = [
    { repositoryName: "vendor/outer", filePath: "vendor/inner/README.md" },
    { repositoryName: "vendor/outer/vendor", filePath: "inner/README.md" },
  ].map((location, index) => ({
    ...location,
    source: "review-file",
    id: String(index),
    sessionId: "s",
    text: "Feedback",
    status: "pending",
    createdAt: "now",
  }));
  render(<ReviewCommentsAttachment comments={comments} />);
  fireEvent.click(screen.getByRole("button"));
  expect(screen.getAllByText("vendor/outer/vendor/inner/README.md")).toHaveLength(2);
});
