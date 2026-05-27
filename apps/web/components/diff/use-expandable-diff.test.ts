import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { FileDiffMetadata } from "@pierre/diffs";

const processFileMock = vi.fn();
const requestFileContentMock = vi.fn();
const requestFileContentAtRefMock = vi.fn();
const getWebSocketClientMock = vi.fn(() => ({}) as unknown);

vi.mock("@pierre/diffs", () => ({
  processFile: (...args: unknown[]) => processFileMock(...args),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClientMock(),
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => requestFileContentMock(...args),
  requestFileContentAtRef: (...args: unknown[]) => requestFileContentAtRefMock(...args),
}));

type UseExpandableDiffFn = typeof import("./use-expandable-diff").useExpandableDiff;

let useExpandableDiff: UseExpandableDiffFn;
beforeEach(async () => {
  vi.resetModules();
  processFileMock.mockReset();
  requestFileContentMock.mockReset();
  requestFileContentAtRefMock.mockReset();
  getWebSocketClientMock.mockReset().mockReturnValue({});
  ({ useExpandableDiff } = await import("./use-expandable-diff"));
});

/** Build a minimal FileDiffMetadata where the trailing-context counts match. */
function consistentMeta(overrides: Partial<FileDiffMetadata> = {}): FileDiffMetadata {
  return {
    name: "src/foo.ts",
    type: "modified" as FileDiffMetadata["type"],
    hunks: [
      {
        additionStart: 1,
        additionCount: 3,
        additionLines: 3,
        additionLineIndex: 0,
        deletionStart: 1,
        deletionCount: 3,
        deletionLines: 3,
        deletionLineIndex: 0,
        hunkContent: [],
      },
    ],
    // 5 lines total, hunk uses 3 starting at 0, leaves 2 trailing on each side
    additionLines: ["a", "b", "c", "d", "e"],
    deletionLines: ["a", "b", "c", "d", "e"],
    splitLineCount: 0,
    unifiedLineCount: 0,
    isPartial: false,
    ...overrides,
  } as FileDiffMetadata;
}

/** Build a FileDiffMetadata whose trailing additions vs deletions disagree. */
function inconsistentMeta(): FileDiffMetadata {
  const m = consistentMeta();
  // Same hunk indices/counts, but extend deletionLines so trailing remainders
  // differ (2 trailing additions vs 4 trailing deletions). This is exactly
  // the shape @pierre/diffs' iterateOverDiff asserts on and throws over.
  return { ...m, deletionLines: [...m.deletionLines, "f", "g"] };
}

const PARTIAL = consistentMeta({ isPartial: true });

const baseProps = {
  sessionId: "sess-1",
  filePath: "src/foo.ts",
  baseRef: "HEAD",
  diff: "diff --git a/src/foo.ts b/src/foo.ts\n--- a/src/foo.ts\n+++ b/src/foo.ts\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n",
  enableExpansion: true,
};

async function loadAndSettle(loadContent: () => Promise<void>) {
  await act(async () => {
    await loadContent();
  });
}

describe("useExpandableDiff", () => {
  it("returns the partial fileDiffMetadata before content is loaded", () => {
    const { result } = renderHook(() =>
      useExpandableDiff({ ...baseProps, fileDiffMetadata: PARTIAL }),
    );
    expect(result.current.metadata).toBe(PARTIAL);
    expect(result.current.isContentLoaded).toBe(false);
  });

  it("returns the reparsed metadata when trailing-context counts agree", async () => {
    requestFileContentMock.mockResolvedValue({ content: "new", is_binary: false });
    requestFileContentAtRefMock.mockResolvedValue({ content: "old", is_binary: false });
    const reparsed = consistentMeta();
    processFileMock.mockReturnValue(reparsed);

    const { result } = renderHook(() =>
      useExpandableDiff({ ...baseProps, fileDiffMetadata: PARTIAL }),
    );
    await loadAndSettle(result.current.loadContent);
    expect(result.current.metadata).toBe(reparsed);
  });

  it("falls back to the partial metadata when the reparse is inconsistent", async () => {
    requestFileContentMock.mockResolvedValue({ content: "new", is_binary: false });
    requestFileContentAtRefMock.mockResolvedValue({ content: "new", is_binary: false });
    processFileMock.mockReturnValue(inconsistentMeta());

    const { result } = renderHook(() =>
      useExpandableDiff({ ...baseProps, fileDiffMetadata: PARTIAL }),
    );
    await loadAndSettle(result.current.loadContent);
    expect(result.current.metadata).toBe(PARTIAL);
  });

  it("preserves the lang override from the partial metadata on a successful reparse", async () => {
    requestFileContentMock.mockResolvedValue({ content: "new", is_binary: false });
    requestFileContentAtRefMock.mockResolvedValue({ content: "old", is_binary: false });
    processFileMock.mockReturnValue(consistentMeta({ lang: "go" as never }));

    const overridden = consistentMeta({ isPartial: true, lang: "text" as never });
    const { result } = renderHook(() =>
      useExpandableDiff({ ...baseProps, fileDiffMetadata: overridden }),
    );
    await loadAndSettle(result.current.loadContent);
    expect((result.current.metadata as { lang?: string }).lang).toBe("text");
  });

  it("returns null when no fileDiffMetadata was provided", () => {
    const { result } = renderHook(() =>
      useExpandableDiff({ ...baseProps, fileDiffMetadata: null }),
    );
    expect(result.current.metadata).toBeNull();
  });
});
