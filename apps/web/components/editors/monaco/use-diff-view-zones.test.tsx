import { cleanup, fireEvent, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useViewZones } from "./use-diff-view-zones";

type FakeZone = { id: string; domNode: HTMLElement };
const NO_COMMENTS: [] = [];
const SELECTED_RANGE = { start: 5, end: 5, side: "additions" };
const COMMENT_PLACEHOLDER = "Add a comment...";
const DRAFT_TEXT = "keep this draft";

function createEditor() {
  const zones = new Map<string, FakeZone>();
  let nextId = 0;
  return {
    zones,
    editor: {
      changeViewZones: (
        update: (accessor: {
          addZone: (zone: { domNode: HTMLElement }) => string;
          removeZone: (id: string) => void;
        }) => void,
      ) => {
        update({
          addZone: ({ domNode }) => {
            const id = `zone-${++nextId}`;
            zones.set(id, { id, domNode });
            document.body.appendChild(domNode);
            return id;
          },
          removeZone: (id) => {
            const zone = zones.get(id);
            zone?.domNode.remove();
            zones.delete(id);
          },
        });
      },
    },
  };
}

afterEach(cleanup);

describe("Monaco comment view-zone readiness", () => {
  it("disables an existing draft during stale display and preserves it until ready", async () => {
    const modified = createEditor();
    const original = createEditor();
    const onSubmit = vi.fn();
    const onSubmitAndRun = vi.fn();
    const args = (submitDisabled: boolean) =>
      ({
        modifiedEditor: modified.editor,
        originalEditor: original.editor,
        comments: NO_COMMENTS,
        showCommentForm: true,
        selectedLineRange: SELECTED_RANGE,
        editingCommentId: null,
        setEditingComment: vi.fn(),
        handleCommentSubmitRef: { current: onSubmit },
        handleCommentSubmitAndRunRef: { current: onSubmitAndRun },
        handleCommentDeleteRef: { current: vi.fn() },
        handleCommentUpdateRef: { current: vi.fn() },
        handleCommentRunRef: { current: vi.fn() },
        clearModifiedGutter: vi.fn(),
        clearOriginalGutter: vi.fn(),
        setShowCommentForm: vi.fn(),
        setSelectedLineRange: vi.fn(),
        submitDisabled,
      }) as unknown as Parameters<typeof useViewZones>[0];

    const view = renderHook(({ submitDisabled }) => useViewZones(args(submitDisabled)), {
      initialProps: { submitDisabled: false },
    });
    const textarea = await screen.findByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(textarea, { target: { value: DRAFT_TEXT } });

    view.rerender({ submitDisabled: true });
    await waitFor(() => {
      expect((screen.getByRole("button", { name: /Add/i }) as HTMLButtonElement).disabled).toBe(
        true,
      );
      expect((screen.getByRole("button", { name: /Run/i }) as HTMLButtonElement).disabled).toBe(
        true,
      );
    });
    expect(screen.getByPlaceholderText(COMMENT_PLACEHOLDER)).toHaveProperty("value", DRAFT_TEXT);
    fireEvent.click(screen.getByRole("button", { name: /Add/i }));
    fireEvent.click(screen.getByRole("button", { name: /Run/i }));
    fireEvent.keyDown(screen.getByPlaceholderText(COMMENT_PLACEHOLDER), {
      key: "Enter",
      ctrlKey: true,
    });
    fireEvent.keyDown(screen.getByPlaceholderText(COMMENT_PLACEHOLDER), {
      key: "Enter",
      ctrlKey: true,
      shiftKey: true,
    });
    expect(onSubmit).not.toHaveBeenCalled();
    expect(onSubmitAndRun).not.toHaveBeenCalled();

    view.rerender({ submitDisabled: false });
    await waitFor(() =>
      expect((screen.getByRole("button", { name: /Run/i }) as HTMLButtonElement).disabled).toBe(
        false,
      ),
    );
    expect(screen.getByPlaceholderText(COMMENT_PLACEHOLDER)).toHaveProperty("value", DRAFT_TEXT);
    fireEvent.click(screen.getByRole("button", { name: /Run/i }));
    expect(onSubmitAndRun).toHaveBeenCalledWith(DRAFT_TEXT);
  });
});
