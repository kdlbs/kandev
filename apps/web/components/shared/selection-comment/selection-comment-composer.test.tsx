import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const touch = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touch.enabled,
}));

import { SelectionCommentSurface } from "./selection-comment-surface";
import { SelectionCommentComposer } from "./selection-comment-composer";

const PLACEHOLDER = "Add your comment or instruction...";
const base = { selectedText: "Buy now", position: { x: 100, y: 100 }, onAdd: vi.fn() };

afterEach(() => {
  touch.enabled = false;
  cleanup();
});

describe("SelectionCommentSurface close control", () => {
  it("shows no close control unless the host opts in", () => {
    render(<SelectionCommentSurface {...base} onClose={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  });

  it("calls onCancel from the desktop header without requiring text", () => {
    const onCancel = vi.fn();
    const onClose = vi.fn();
    render(<SelectionCommentSurface {...base} onClose={onClose} onCancel={onCancel} />);

    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("offers a touch-sized Close button in the drawer", () => {
    touch.enabled = true;
    const onCancel = vi.fn();
    render(<SelectionCommentSurface {...base} onClose={vi.fn()} onCancel={onCancel} />);

    const close = screen.getByRole("button", { name: "Close" });
    expect(close.className).toContain("min-h-11");
    fireEvent.click(close);
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("keeps Add disabled while the host is not ready, even with text", () => {
    render(<SelectionCommentSurface {...base} onClose={vi.fn()} addDisabled />);
    fireEvent.change(screen.getByPlaceholderText(PLACEHOLDER), { target: { value: "Bigger" } });
    expect((screen.getByRole("button", { name: "Add" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("SelectionCommentSurface drag", () => {
  it("covers embedded frames with a shield only while the header is dragged", () => {
    render(<SelectionCommentSurface {...base} onClose={vi.fn()} />);
    const handle = document.querySelector("[data-selection-comment-drag-handle]");
    expect(handle).not.toBeNull();

    fireEvent.mouseDown(handle as Element, { clientX: 10, clientY: 10 });
    expect(document.querySelector("[data-selection-comment-drag-shield]")).not.toBeNull();

    fireEvent.mouseUp(document, { clientX: 40, clientY: 40 });
    expect(document.querySelector("[data-selection-comment-drag-shield]")).toBeNull();
  });
});

describe("SelectionCommentComposer", () => {
  it("renders inline, saves, and closes like the floating surface", async () => {
    const onAdd = vi.fn().mockResolvedValue(true);
    const onClose = vi.fn();
    render(
      <SelectionCommentComposer
        selectedText="Buy now"
        onAdd={onAdd}
        onClose={onClose}
        onCancel={vi.fn()}
        testId="inline"
      />,
    );

    const container = screen.getByTestId("inline");
    expect(container.closest("[data-selection-comment-surface]")).not.toBeNull();
    expect(document.activeElement).toBe(screen.getByPlaceholderText(PLACEHOLDER));
    fireEvent.change(screen.getByPlaceholderText(PLACEHOLDER), { target: { value: "Bigger" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(onAdd).toHaveBeenCalledWith("Bigger", "Buy now");
    expect(screen.getByRole("button", { name: "Close" })).toBeTruthy();
  });

  it("shows the context label and closes on Escape", () => {
    const onClose = vi.fn();
    render(
      <SelectionCommentComposer
        selectedText="Buy now"
        contextLabel="/checkout"
        onAdd={vi.fn()}
        onClose={onClose}
      />,
    );
    expect(screen.getByTestId("selection-comment-context-label").textContent).toBe("/checkout");
    fireEvent.keyDown(screen.getByPlaceholderText(PLACEHOLDER), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
