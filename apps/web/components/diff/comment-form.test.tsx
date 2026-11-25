import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CommentForm } from "./comment-form";
afterEach(cleanup);
it("preserves input when saving is rejected and clears accepted legacy saves", () => {
  const onSubmit = vi.fn(() => false);
  const { rerender } = render(<CommentForm onSubmit={onSubmit} />);
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Keep this" } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Keep this");
  rerender(<CommentForm onSubmit={() => {}} />);
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
});
it("edits a controlled draft and stops Escape at the composer", () => {
  const onChange = vi.fn();
  const onEscapeOutside = vi.fn();
  const onCancel = vi.fn();
  render(
    <div onKeyDown={onEscapeOutside}>
      <CommentForm value="Draft" onChange={onChange} onSubmit={() => {}} onCancel={onCancel} />
    </div>,
  );
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Draft");
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "Changed" } });
  expect(onChange).toHaveBeenCalledWith("Changed");
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
  expect(onCancel).toHaveBeenCalledOnce();
  expect(onEscapeOutside).not.toHaveBeenCalled();
});
