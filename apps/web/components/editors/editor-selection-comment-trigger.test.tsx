import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EditorSelectionCommentTrigger } from "./editor-selection-comment-trigger";

afterEach(cleanup);

describe("EditorSelectionCommentTrigger", () => {
  it("floats beside the selection with a fine pointer", () => {
    const onOpen = vi.fn();
    render(
      <EditorSelectionCommentTrigger
        touch={false}
        position={{ left: 44, top: 62 }}
        positioning="absolute"
        onOpen={onOpen}
      />,
    );

    const button = screen.getByTestId("code-editor-comment-button");
    expect(button.classList.contains("floating-comment-btn")).toBe(true);
    expect(button.classList.contains("absolute")).toBe(true);
    expect(button.style.left).toBe("44px");
    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("docks in a toolbar on touch", () => {
    const onOpen = vi.fn();
    render(
      <EditorSelectionCommentTrigger
        touch
        position={{ left: 44, top: 62 }}
        positioning="fixed"
        onOpen={onOpen}
      />,
    );

    const toolbar = screen.getByRole("toolbar", { name: "Selection actions" });
    const button = screen.getByRole("button", { name: "Comment on selection" });
    expect(button.parentElement).toBe(toolbar);
    expect(button.classList.contains("floating-comment-btn")).toBe(false);
    fireEvent.click(button);
    expect(onOpen).toHaveBeenCalledTimes(1);
  });
});
