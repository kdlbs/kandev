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

describe("useKanbanPreview displacement", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  const OPEN_KEY = "kandev.kanban.preview.open";
  const TASK_KEY = "kandev.kanban.preview.selectedTask";

  it("hides a saved preview while displaced without persisting over or losing it", async () => {
    window.localStorage.setItem(OPEN_KEY, "true");
    window.localStorage.setItem(TASK_KEY, JSON.stringify("t-1"));
    const { result } = renderHook(() => useKanbanPreview());
    await waitFor(() => expect(result.current.isOpen).toBe(true));

    act(() => result.current.displace(true));

    expect(result.current.isOpen).toBe(false);
    expect(result.current.selectedTaskId).toBe("t-1");
    expect(window.localStorage.getItem(OPEN_KEY)).toBe("true");
    expect(window.localStorage.getItem(TASK_KEY)).toBe(JSON.stringify("t-1"));

    act(() => result.current.displace(false));
    expect(result.current.isOpen).toBe(true);
    expect(result.current.selectedTaskId).toBe("t-1");
  });

  it("keeps a saved preview when the hook mounts already displaced", async () => {
    window.localStorage.setItem(OPEN_KEY, "true");
    window.localStorage.setItem(TASK_KEY, JSON.stringify("t-1"));
    const { result } = renderHook(() => useKanbanPreview({ displaced: true }));
    await waitFor(() => expect(result.current.selectedTaskId).toBe("t-1"));

    expect(result.current.isOpen).toBe(false);
    expect(window.localStorage.getItem(OPEN_KEY)).toBe("true");
    expect(window.localStorage.getItem(TASK_KEY)).toBe(JSON.stringify("t-1"));
  });

  it("an explicit open while displaced shows the new task", async () => {
    const { result } = renderHook(() => useKanbanPreview({ displaced: true }));
    act(() => result.current.open("t-2"));
    expect(result.current.isOpen).toBe(true);
    expect(result.current.selectedTaskId).toBe("t-2");
  });
});
