import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const touch = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touch.enabled,
}));

import { SelectionCommentSurface } from "./selection-comment-surface";

const SAVED_FEEDBACK = "Keep this feedback";
const COMMENT_PLACEHOLDER = "Add your comment or instruction...";

afterEach(() => {
  touch.enabled = false;
  cleanup();
});

// eslint-disable-next-line max-lines-per-function -- Desktop and touch popover contracts share one render harness.
describe("SelectionCommentSurface", () => {
  it("keeps entered feedback open when the submitter rejects the save", () => {
    const onClose = vi.fn();
    render(
      <SelectionCommentSurface
        selectedText="settled answer"
        position={{ x: 100, y: 100 }}
        onAdd={() => false}
        onClose={onClose}
      />,
    );

    const input = screen.getByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    expect(onClose).not.toHaveBeenCalled();
    expect((input as HTMLTextAreaElement).value).toBe(SAVED_FEEDBACK);
  });

  it("waits for persistence and keeps entered feedback when the async save fails", async () => {
    const onClose = vi.fn();
    const onAdd = vi.fn().mockResolvedValue(false);
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={onAdd}
        onClose={onClose}
        errorMessage="Could not save this comment. Try again."
      />,
    );

    const input = screen.getByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onAdd).toHaveBeenCalled());
    expect(onClose).not.toHaveBeenCalled();
    expect((input as HTMLTextAreaElement).value).toBe(SAVED_FEEDBACK);
    expect(screen.getByRole("alert").textContent).toContain("Could not save this comment");
  });

  it("disables Run and exposes the primary-session reason", () => {
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={() => true}
        onAddAndRun={() => true}
        runDisabledReason="This task has no primary session available for Run."
        onClose={() => undefined}
      />,
    );

    expect(screen.getByRole("button", { name: "Run" }).hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("This task has no primary session available for Run.")).toBeTruthy();
  });

  it("keeps an edited comment open when async deletion is rejected", async () => {
    const onClose = vi.fn();
    const onDelete = vi.fn().mockResolvedValue(false);
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        editingComment="Keep this"
        onAdd={() => true}
        onDelete={onDelete}
        onClose={onClose}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete comment" }));

    await waitFor(() => expect(onDelete).toHaveBeenCalled());
    expect(onClose).not.toHaveBeenCalled();
  });

  it("uses a scroll-contained drawer with touch-sized actions on coarse pointers", () => {
    touch.enabled = true;
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={() => true}
        onAddAndRun={() => true}
        onClose={() => undefined}
      />,
    );

    expect(screen.getByTestId("plan-comment-drawer").className).toContain("max-h-[82dvh]");
    expect(screen.getByRole("button", { name: "Add" }).className).toContain("min-h-11");
    expect(screen.getByRole("button", { name: "Run" }).className).toContain("min-h-11");
  });

  it("ignores repeated keyboard submission events", () => {
    const onAdd = vi.fn();
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={onAdd}
        onClose={() => undefined}
      />,
    );
    const input = screen.getByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });

    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true, repeat: true });

    expect(onAdd).not.toHaveBeenCalled();
  });

  it("disables delete while an edit submission is pending", async () => {
    let finish!: () => void;
    const onAdd = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        editingComment={SAVED_FEEDBACK}
        onAdd={onAdd}
        onDelete={() => true}
        onClose={() => undefined}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Update" }));

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Delete comment" }).hasAttribute("disabled")).toBe(
        true,
      ),
    );
    finish();
  });

  it("ignores desktop dismissal while a save is pending", async () => {
    let finish!: (accepted: boolean) => void;
    const onClose = vi.fn();
    const onAdd = vi.fn(() => new Promise<boolean>((resolve) => (finish = resolve)));
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={onAdd}
        onClose={onClose}
      />,
    );
    const input = screen.getByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => expect(onAdd).toHaveBeenCalled());

    fireEvent.keyDown(document, { key: "Escape" });
    fireEvent.mouseDown(document.body);

    expect(onClose).not.toHaveBeenCalled();
    await act(async () => finish(false));
  });

  it("ignores touch drawer dismissal while a save is pending", async () => {
    touch.enabled = true;
    let finish!: (accepted: boolean) => void;
    const onClose = vi.fn();
    const onAdd = vi.fn(() => new Promise<boolean>((resolve) => (finish = resolve)));
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        onAdd={onAdd}
        onClose={onClose}
      />,
    );
    const input = screen.getByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(input, { target: { value: SAVED_FEEDBACK } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => expect(onAdd).toHaveBeenCalled());

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onClose).not.toHaveBeenCalled();
    await act(async () => finish(false));
  });

  it("ignores desktop dismissal while deletion is pending", async () => {
    let finish!: (accepted: boolean) => void;
    const onClose = vi.fn();
    const onDelete = vi.fn(() => new Promise<boolean>((resolve) => (finish = resolve)));
    render(
      <SelectionCommentSurface
        selectedText="plan step"
        position={{ x: 100, y: 100 }}
        editingComment={SAVED_FEEDBACK}
        onAdd={() => true}
        onDelete={onDelete}
        onClose={onClose}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Delete comment" }));
    await waitFor(() => expect(onDelete).toHaveBeenCalled());

    fireEvent.keyDown(document, { key: "Escape" });
    fireEvent.mouseDown(document.body);

    expect(onClose).not.toHaveBeenCalled();
    await act(async () => finish(false));
  });
});

// @covers AC-UI-SELECTION-COMMENT-SURFACE-001.4
// @covers AC-UI-SELECTION-COMMENT-SURFACE-001.7
// @covers AC-UI-SELECTION-COMMENT-SURFACE-001.8
describe("SelectionCommentSurface shared extensions", () => {
  it("shows a context label and custom evidence instead of the quoted text", () => {
    render(
      <SelectionCommentSurface
        selectedText="plain evidence"
        contextLabel="Lines 3-5"
        evidence={<img alt="Screenshot preview" src="blob:shot" />}
        position={{ x: 10, y: 10 }}
        onAdd={() => {}}
        onClose={() => {}}
      />,
    );
    expect(screen.getByTestId("selection-comment-context-label").textContent).toBe("Lines 3-5");
    expect(screen.getByRole("img", { name: "Screenshot preview" })).toBeTruthy();
    expect(screen.queryByText(/plain evidence/)).toBeNull();
  });

  it("shows the context label and evidence in the touch drawer", () => {
    touch.enabled = true;
    render(
      <SelectionCommentSurface
        selectedText="plain evidence"
        contextLabel="/pricing"
        evidence={<span>thumbnail</span>}
        position={{ x: 10, y: 10 }}
        onAdd={() => {}}
        onClose={() => {}}
      />,
    );
    expect(screen.getByTestId("selection-comment-context-label").textContent).toBe("/pricing");
    expect(screen.getByText("thumbnail")).toBeTruthy();
  });

  it("uses a controlled draft that survives closing and reopening", () => {
    let draft = "";
    const onDraftChange = vi.fn((value: string) => {
      draft = value;
    });
    const props = {
      selectedText: "x",
      position: { x: 10, y: 10 },
      onAdd: () => {},
      onClose: () => {},
      onDraftChange,
    };
    const { unmount } = render(<SelectionCommentSurface {...props} draftComment={draft} />);
    fireEvent.change(screen.getByPlaceholderText(COMMENT_PLACEHOLDER), {
      target: { value: "kept text" },
    });
    expect(onDraftChange).toHaveBeenCalledWith("kept text");
    unmount();

    render(<SelectionCommentSurface {...props} draftComment={draft} />);
    expect((screen.getByPlaceholderText(COMMENT_PLACEHOLDER) as HTMLTextAreaElement).value).toBe(
      "kept text",
    );
  });

  it("stays inside the viewport after the window shrinks", () => {
    const previous = Object.getOwnPropertyDescriptor(window, "innerHeight");
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 667 });
    try {
      render(
        <SelectionCommentSurface
          selectedText="x"
          position={{ x: 40, y: 600 }}
          onAdd={() => {}}
          onClose={() => {}}
          testId="surface"
        />,
      );
      fireEvent.change(screen.getByPlaceholderText(COMMENT_PLACEHOLDER), {
        target: { value: "keep this note" },
      });
      Object.defineProperty(window, "innerHeight", { configurable: true, value: 375 });
      act(() => {
        window.dispatchEvent(new Event("resize"));
      });
      const top = Number.parseFloat(screen.getByTestId("surface").style.top);
      expect(top + 180).toBeLessThanOrEqual(375);
      expect((screen.getByPlaceholderText(COMMENT_PLACEHOLDER) as HTMLTextAreaElement).value).toBe(
        "keep this note",
      );
    } finally {
      if (previous) Object.defineProperty(window, "innerHeight", previous);
    }
  });
});

describe("SelectionCommentSurface placement", () => {
  it("keeps a tall surface, such as one with a thumbnail, inside the viewport", () => {
    const previous = Object.getOwnPropertyDescriptor(window, "innerHeight");
    const original = HTMLElement.prototype.getBoundingClientRect;
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 800 });
    HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
      const rect = original.call(this);
      if (this.dataset.testid !== "tall-surface") return rect;
      return { ...rect.toJSON(), height: 420, width: 340, bottom: rect.top + 420 } as DOMRect;
    };
    try {
      render(
        <SelectionCommentSurface
          selectedText="x"
          evidence={<div style={{ height: 400 }}>thumbnail</div>}
          position={{ x: 40, y: 500 }}
          onAdd={() => {}}
          onClose={() => {}}
          testId="tall-surface"
        />,
      );
      const top = Number.parseFloat(screen.getByTestId("tall-surface").style.top);
      expect(top + 420).toBeLessThanOrEqual(800);
    } finally {
      HTMLElement.prototype.getBoundingClientRect = original;
      if (previous) Object.defineProperty(window, "innerHeight", previous);
    }
  });

  it("animates in by default and can appear in place without animation", () => {
    const surface = (animated?: boolean) =>
      render(
        <SelectionCommentSurface
          selectedText="target"
          position={{ x: 10, y: 10 }}
          onAdd={() => {}}
          onClose={() => {}}
          testId="surface"
          animated={animated}
        />,
      );
    surface();
    expect(screen.getByTestId("surface").className).toContain("animate-in");
    cleanup();
    surface(false);
    expect(screen.getByTestId("surface").className).not.toContain("animate-in");
  });
});
