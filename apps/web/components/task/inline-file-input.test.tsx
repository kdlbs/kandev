import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { InlineFileInput } from "./inline-file-input";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-UI-EDIT-FOCUS-001.1, AC-UI-EDIT-FOCUS-001.4
it("does not create an unfinished file when the document loses focus", () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  const submit = vi.fn();
  const cancel = vi.fn();
  render(<InlineFileInput depth={0} onSubmit={submit} onCancel={cancel} />);
  const input = screen.getByRole("textbox") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "draft.ts" } });
  fireEvent.blur(input);
  expect(submit).not.toHaveBeenCalled();
  expect(cancel).not.toHaveBeenCalled();
  expect(input.value).toBe("draft.ts");
  fireEvent.keyDown(input, { key: "Enter" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(submit).toHaveBeenCalledExactlyOnceWith("draft.ts");
});

it("does not create a file after Escape followed by blur", () => {
  const submit = vi.fn();
  const cancel = vi.fn();
  render(<InlineFileInput depth={0} onSubmit={submit} onCancel={cancel} />);
  const input = screen.getByRole("textbox");
  fireEvent.change(input, { target: { value: "discard.ts" } });
  fireEvent.keyDown(input, { key: "Escape" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(cancel).toHaveBeenCalledTimes(1);
  expect(submit).not.toHaveBeenCalled();
});
