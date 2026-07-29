import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { OpenFileTab } from "@/lib/types/backend";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const fetchAndOpenFileMock = vi.fn();
vi.mock("../file-browser-hooks", () => ({
  fetchAndOpenFile: (...args: unknown[]) => fetchAndOpenFileMock(...args),
}));

import { useTabletReviewPreview } from "./use-tablet-review-preview";

describe("useTabletReviewPreview", () => {
  beforeEach(() => fetchAndOpenFileMock.mockReset());

  it("opens the selected repository's Markdown file in preview mode", () => {
    const onOpenFile = vi.fn();
    const { result } = renderHook(() => useTabletReviewPreview("session-1", onOpenFile));

    act(() => result.current("README.md", "frontend"));

    expect(fetchAndOpenFileMock).toHaveBeenCalledWith(
      "session-1",
      "README.md",
      expect.any(Function),
      expect.any(Function),
      { repo: "frontend", signal: expect.objectContaining({ aborted: false }) },
    );
    const opened = fetchAndOpenFileMock.mock.calls[0]?.[2] as (file: OpenFileTab) => void;
    act(() => opened({ path: "README.md" } as OpenFileTab));
    expect(onOpenFile).toHaveBeenCalledWith({ path: "README.md", markdownPreview: true });
  });

  it("ignores a response after the active session changes", () => {
    const onOpenFile = vi.fn();
    const { result, rerender } = renderHook(
      ({ sessionId }) => useTabletReviewPreview(sessionId, onOpenFile),
      { initialProps: { sessionId: "session-1" } },
    );
    act(() => result.current("README.md", "frontend"));
    const options = fetchAndOpenFileMock.mock.calls[0]?.[4] as { signal: AbortSignal };
    const opened = fetchAndOpenFileMock.mock.calls[0]?.[2] as (file: OpenFileTab) => void;

    rerender({ sessionId: "session-2" });
    act(() => opened({ path: "README.md" } as OpenFileTab));

    expect(options.signal.aborted).toBe(true);
    expect(onOpenFile).not.toHaveBeenCalled();
  });
});
