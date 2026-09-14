import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { EditorActions } from "./threads-view-editor-actions";

describe("EditorActions", () => {
  afterEach(cleanup);

  it("anchors delete requests and lets inline confirmation replace the action row", () => {
    const onDelete = vi.fn();
    const { rerender } = render(
      <EditorActions
        hasDraft={false}
        canDelete
        viewCount={2}
        invalidDraft={false}
        onSave={vi.fn()}
        onSaveAs={vi.fn()}
        onDiscard={vi.fn()}
        onDelete={onDelete}
      />,
    );

    fireEvent.click(screen.getByTestId("threads-view-delete"));
    expect(onDelete).toHaveBeenCalledOnce();

    rerender(
      <EditorActions
        hasDraft={false}
        canDelete
        viewCount={2}
        invalidDraft={false}
        deleteConfirmation={<div data-testid="inline-delete-confirmation" />}
        onSave={vi.fn()}
        onSaveAs={vi.fn()}
        onDiscard={vi.fn()}
        onDelete={onDelete}
      />,
    );

    expect(screen.getByTestId("inline-delete-confirmation")).toBeTruthy();
    expect(screen.queryByTestId("threads-view-delete")).toBeNull();
  });
});
