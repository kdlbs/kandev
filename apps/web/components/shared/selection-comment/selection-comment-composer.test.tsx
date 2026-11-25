import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const touch = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touch.enabled,
}));

import { SelectionCommentSurface } from "./selection-comment-surface";

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
