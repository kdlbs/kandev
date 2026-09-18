import { useState } from "react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ReviewFileComments } from "./review-file-comments";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { ReviewFile } from "./types";

const file: ReviewFile = {
  path: "README.md",
  repository_name: "api",
  status: "deleted",
  source: "uncommitted",
  staged: false,
  additions: 0,
  deletions: 0,
  diff: "",
};
function Harness() {
  const [creating, setCreating] = useState(true);
  return (
    <ReviewFileComments
      file={file}
      sessionId="s"
      creating={creating}
      onClose={() => setCreating(false)}
    />
  );
}
beforeEach(() => {
  sessionStorage.clear();
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
});
afterEach(cleanup);
it("creates, edits and deletes a comment on a patchless deleted file", () => {
  render(<Harness />);
  expect(screen.getByRole("button", { name: "Add" })).toHaveProperty("disabled", true);
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Please keep this file" } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  expect(screen.getByText("Please keep this file")).toBeDefined();
  expect(useCommentsStore.getState().getPendingComments()).toEqual([
    expect.objectContaining({
      source: "review-file",
      repositoryName: "api",
      filePath: "README.md",
    }),
  ]);
  fireEvent.click(screen.getByRole("button", { name: "Edit comment" }));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Updated feedback" } });
  fireEvent.click(screen.getByRole("button", { name: "Update" }));
  expect(screen.getByText("Updated feedback")).toBeDefined();
  fireEvent.click(screen.getByRole("button", { name: "Delete comment" }));
  expect(useCommentsStore.getState().getPendingComments()).toEqual([]);
});
it("cancels a draft with Escape without creating feedback", () => {
  render(<Harness />);
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Draft" } });
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(useCommentsStore.getState().getPendingComments()).toEqual([]);
});
