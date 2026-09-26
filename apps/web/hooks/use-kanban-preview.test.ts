import { renderHook, waitFor, act } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useKanbanPreview } from "./use-kanban-preview";
import { PREVIEW_PANEL } from "@/lib/settings/constants";

describe("useKanbanPreview minimum width", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("floors a persisted width below the fine-pointer minimum on restore", async () => {
    window.localStorage.setItem("kandev.kanban.preview.width", "1");

    const { result } = renderHook(() => useKanbanPreview());

    await waitFor(() => expect(result.current.previewWidthPx).toBe(PREVIEW_PANEL.MIN_WIDTH_PX));
  });

  it("floors updatePreviewWidth to the fine-pointer minimum", async () => {
    const { result } = renderHook(() => useKanbanPreview());
    await waitFor(() => expect(result.current.previewWidthPx).toBe(PREVIEW_PANEL.DEFAULT_WIDTH_PX));

    act(() => result.current.updatePreviewWidth(1));

    expect(result.current.previewWidthPx).toBe(PREVIEW_PANEL.MIN_WIDTH_PX);
  });
});
