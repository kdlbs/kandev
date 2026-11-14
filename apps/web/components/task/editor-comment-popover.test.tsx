import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { EditorCommentPopover } from "./editor-comment-popover";

let previousInnerHeight: PropertyDescriptor | undefined;

afterEach(() => {
  cleanup();
  if (previousInnerHeight) {
    Object.defineProperty(window, "innerHeight", previousInnerHeight);
  }
});

describe("EditorCommentPopover viewport changes", () => {
  it("keeps the draft and actions reachable after the viewport height shrinks", () => {
    previousInnerHeight = Object.getOwnPropertyDescriptor(window, "innerHeight");
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 667 });

    const { container } = render(
      <EditorCommentPopover
        selectedText="selected text"
        lineRange={{ start: 3, end: 3 }}
        position={{ x: 40, y: 600 }}
        onSubmit={() => {}}
        onSubmitAndRun={() => {}}
        onClose={() => {}}
      />,
    );
    const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
    const popover = container.querySelector<HTMLElement>(".fixed");
    expect(popover?.style.top).toBe("380px");

    fireEvent.change(textarea, { target: { value: "keep this note" } });
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 375 });
    fireEvent(window, new Event("resize"));

    expect(textarea.value).toBe("keep this note");
    expect(popover).not.toBeNull();
    const top = Number.parseFloat(popover!.style.top);
    expect(top + 220).toBeLessThanOrEqual(window.innerHeight - 16);
    expect(
      screen.getAllByRole("button").every((button) => !(button as HTMLButtonElement).disabled),
    ).toBe(true);
  });
});
