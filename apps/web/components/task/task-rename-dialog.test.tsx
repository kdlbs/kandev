import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TaskRenameDialog } from "./task-rename-dialog";

const originalAnimations = Object.getOwnPropertyDescriptor(Element.prototype, "getAnimations");

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalAnimations)
    Object.defineProperty(Element.prototype, "getAnimations", originalAnimations);
  else Reflect.deleteProperty(Element.prototype, "getAnimations");
});

// @covers AC-UI-EDIT-FOCUS-001.2
it("keeps the draft selection when the Rename dialog regains focus", () => {
  Object.defineProperty(Element.prototype, "getAnimations", {
    configurable: true,
    value: () => [],
  });
  const submit = vi.fn();
  render(
    <TaskRenameDialog
      open
      currentTitle="Original title"
      onOpenChange={vi.fn()}
      onSubmit={submit}
    />,
  );
  const input = screen.getByRole("textbox") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "Release notes" } });
  input.setSelectionRange(7, 7);
  fireEvent.focus(input);
  expect([input.selectionStart, input.selectionEnd]).toEqual([7, 7]);
  expect(input.value).toBe("Release notes");
  expect(submit).not.toHaveBeenCalled();
});
