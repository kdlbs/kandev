import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TabRenameInput } from "./tab-rename-input";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-UI-EDIT-FOCUS-001.1
it("does not commit a tab rename when the document loses focus", () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  const commit = vi.fn();
  render(<TabRenameInput initial="Original" seqBadge={1} onCommit={commit} onCancel={vi.fn()} />);
  const input = screen.getByRole("textbox") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "Unfinished name" } });
  fireEvent.blur(input);
  expect(commit).not.toHaveBeenCalled();
  expect(input.value).toBe("Unfinished name");
  fireEvent.keyDown(input, { key: "Enter" });
  fireEvent.keyDown(input, { key: "Enter" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(commit).toHaveBeenCalledExactlyOnceWith("Unfinished name");
});

it("cancels on Escape without a subsequent blur commit", () => {
  const commit = vi.fn();
  const cancel = vi.fn();
  render(<TabRenameInput initial="Original" seqBadge={null} onCommit={commit} onCancel={cancel} />);
  const input = screen.getByRole("textbox");
  fireEvent.keyDown(input, { key: "Escape" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(commit).not.toHaveBeenCalled();
  expect(cancel).toHaveBeenCalledTimes(1);
});
