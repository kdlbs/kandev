import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mockOpenSessionInEditor = vi.fn();
const mockOpenFileInVscode = vi.fn();
const mockOpenInternalVscode = vi.fn();
const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api", () => ({
  openSessionInEditor: (...args: unknown[]) => mockOpenSessionInEditor(...args),
}));

vi.mock("@/lib/api/domains/vscode-api", () => ({
  openFileInVscode: (...args: unknown[]) => mockOpenFileInVscode(...args),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ openInternalVscode: mockOpenInternalVscode }),
  },
}));

vi.mock("@/lib/http/use-request", () => ({
  useRequest: (fn: (options?: unknown) => Promise<unknown>) => ({
    run: fn,
    status: "idle",
    isLoading: false,
  }),
}));

import { useOpenSessionInEditor } from "./use-open-session-in-editor";

describe("useOpenSessionInEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockOpenSessionInEditor.mockResolvedValue({});
  });

  it("forwards the selected worktree as worktree_id", async () => {
    const { result } = renderHook(() => useOpenSessionInEditor("session-1"));

    await result.current.open({ editorId: "editor-1", worktreeId: "wt-2" });

    expect(mockOpenSessionInEditor).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ editor_id: "editor-1", worktree_id: "wt-2" }),
      expect.anything(),
    );
  });

  it("omits worktree_id when no worktree is selected", async () => {
    const { result } = renderHook(() => useOpenSessionInEditor("session-1"));

    await result.current.open({ editorId: "editor-1" });

    const payload = mockOpenSessionInEditor.mock.calls[0][1] as Record<string, unknown>;
    expect(payload.worktree_id).toBeUndefined();
  });

  it("does not call the API without a session id", async () => {
    const { result } = renderHook(() => useOpenSessionInEditor(null));

    await result.current.open({ editorId: "editor-1", worktreeId: "wt-2" });

    expect(mockOpenSessionInEditor).not.toHaveBeenCalled();
  });

  it("reports editor API failures instead of leaving the button silently idle", async () => {
    mockOpenSessionInEditor.mockRejectedValueOnce(new Error("workspace path not found"));
    const { result } = renderHook(() => useOpenSessionInEditor("session-1"));

    await expect(result.current.open({ editorId: "editor-1" })).resolves.toBeNull();

    expect(mockToast).toHaveBeenCalledWith({
      title: "Failed to open editor",
      description: "workspace path not found",
      variant: "error",
    });
  });
});
