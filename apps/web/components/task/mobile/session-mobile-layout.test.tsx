import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, renderHook, act, screen } from "@testing-library/react";
import { useState } from "react";
import type { OpenFileTab } from "@/lib/types/backend";
import type { ReviewItemSummary } from "@/lib/plugins/types";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const fetchAndOpenFileMock = vi.fn();
vi.mock("../file-browser-hooks", () => ({
  fetchAndOpenFile: (...args: unknown[]) => fetchAndOpenFileMock(...args),
}));

vi.mock("@/hooks/use-visual-viewport-offset", () => ({
  useVisualViewportOffset: () => ({ keyboardOpen: false, bottomOffset: 0 }),
}));

vi.mock("../review-item-selector", () => ({
  ReviewItemSelector: ({
    reviews,
    onSelectReview,
  }: {
    reviews: ReviewItemSummary[];
    onSelectReview: (review: ReviewItemSummary) => void;
  }) => (
    <button type="button" onClick={() => onSelectReview(reviews[1]!)}>
      Select review B
    </button>
  ),
}));

vi.mock("../review-detail-panel", async () => {
  const React = await import("react");
  return {
    ReviewDetailPanelComponent: ({ params }: { params: { reviewKey: string } }) => {
      const [feedback] = React.useState(`feedback for ${params.reviewKey}`);
      return <button type="button">{feedback}</button>;
    },
  };
});

import {
  MobilePanelArea,
  useMobilePanelHandlers,
  useMobileReviewPanelFallback,
} from "./session-mobile-layout";

const MOCK_FILE: OpenFileTab = {
  path: "src/foo.ts",
  name: "foo.ts",
  content: "export const x = 1;",
  originalContent: "export const x = 1;",
  originalHash: "abc123",
  isDirty: false,
};

const OTHER_FILE: OpenFileTab = {
  ...MOCK_FILE,
  path: "src/bar.ts",
  name: "bar.ts",
};

const CHAT_LINK_PATH = "src/chat-link.ts";
const REPO = "frontend";

function renderHandlers(initialSid: string | null = "s1") {
  const handlePanelChange = vi.fn();
  const view = renderHook(
    ({ sid }) => useMobilePanelHandlers({ effectiveSessionId: sid, handlePanelChange }),
    { initialProps: { sid: initialSid } },
  );
  return { handlePanelChange, ...view };
}

describe("useMobilePanelHandlers", () => {
  beforeEach(() => {
    fetchAndOpenFileMock.mockReset();
  });

  it("handleOpenFile sets selectedFile and switches to files panel", () => {
    const { result, handlePanelChange } = renderHandlers();
    expect(result.current.selectedFile).toBeNull();

    act(() => result.current.handleOpenFile(MOCK_FILE));

    expect(result.current.selectedFile).toEqual(MOCK_FILE);
    expect(handlePanelChange).toHaveBeenCalledWith("files");
  });

  it("handleOpenFileFromChat fetches and opens the viewer panel", () => {
    const { result, handlePanelChange } = renderHandlers();
    act(() => result.current.handleOpenFileFromChat(CHAT_LINK_PATH));

    expect(fetchAndOpenFileMock).toHaveBeenCalledWith(
      "s1",
      CHAT_LINK_PATH,
      expect.any(Function),
      expect.any(Function),
      { repo: undefined, signal: expect.objectContaining({ aborted: false }) },
    );

    const openFile = fetchAndOpenFileMock.mock.calls[0]?.[2] as (file: OpenFileTab) => void;
    act(() => openFile(MOCK_FILE));

    expect(result.current.selectedFile).toEqual(MOCK_FILE);
    expect(handlePanelChange).toHaveBeenCalledWith("files");
  });

  it("opens a fetched Markdown file with preview intent", () => {
    const { result, handlePanelChange } = renderHandlers();
    act(() => result.current.handleOpenFileFromChat("README.md", REPO, true));

    expect(fetchAndOpenFileMock).toHaveBeenCalledWith(
      "s1",
      "README.md",
      expect.any(Function),
      expect.any(Function),
      { repo: REPO, signal: expect.objectContaining({ aborted: false }) },
    );

    const openFile = fetchAndOpenFileMock.mock.calls[0]?.[2] as (file: OpenFileTab) => void;
    act(() => openFile({ ...MOCK_FILE, path: "README.md", name: "README.md" }));

    expect(result.current.selectedFile).toMatchObject({ path: "README.md" });
    expect(result.current.selectedFilePreview).toBe(true);
    expect(handlePanelChange).toHaveBeenCalledWith("files");
  });

  it("passes repo through when opening a walkthrough file from mobile", () => {
    const { result } = renderHandlers();
    act(() => result.current.handleOpenFileFromChat(CHAT_LINK_PATH, REPO));

    expect(fetchAndOpenFileMock).toHaveBeenCalledWith(
      "s1",
      CHAT_LINK_PATH,
      expect.any(Function),
      expect.any(Function),
      { repo: REPO, signal: expect.objectContaining({ aborted: false }) },
    );
  });

  it("handleOpenFileFromChat no-ops when no active session", () => {
    const { result } = renderHandlers(null);
    act(() => result.current.handleOpenFileFromChat(CHAT_LINK_PATH));

    expect(fetchAndOpenFileMock).not.toHaveBeenCalled();
    expect(result.current.selectedFile).toBeNull();
  });
});

describe("useMobilePanelHandlers selection state", () => {
  beforeEach(() => {
    fetchAndOpenFileMock.mockReset();
  });

  it("handlePanelChangeAndClearSheet clears the viewer when switching panels", () => {
    const { result, handlePanelChange } = renderHandlers();
    act(() => result.current.handleOpenFile(MOCK_FILE));
    expect(result.current.selectedFile).toEqual(MOCK_FILE);

    act(() => result.current.handlePanelChangeAndClearSheet("plan"));

    expect(result.current.selectedFile).toBeNull();
    expect(handlePanelChange).toHaveBeenCalledWith("plan");
  });

  it("clears selectedFile when effectiveSessionId changes", () => {
    const { result, rerender } = renderHandlers();
    act(() => result.current.handleOpenFile(MOCK_FILE));
    expect(result.current.selectedFile).toEqual(MOCK_FILE);

    rerender({ sid: "s2" });
    expect(result.current.selectedFile).toBeNull();
  });

  it("keeps selectedFile when rerendered with same session", () => {
    const { result, rerender } = renderHandlers();
    act(() => result.current.handleOpenFile(MOCK_FILE));
    rerender({ sid: "s1" });
    expect(result.current.selectedFile).toEqual(MOCK_FILE);
  });

  it("rejects stale handleOpenFileFromChat callback after session change", () => {
    const { result, rerender } = renderHandlers();
    act(() => result.current.handleOpenFileFromChat(CHAT_LINK_PATH));

    expect(fetchAndOpenFileMock).toHaveBeenCalledWith(
      "s1",
      CHAT_LINK_PATH,
      expect.any(Function),
      expect.any(Function),
      { repo: undefined, signal: expect.objectContaining({ aborted: false }) },
    );

    // Simulate session switch before the async callback fires
    rerender({ sid: "s2" });
    expect(result.current.selectedFile).toBeNull();

    // Invoke the stale callback that was registered for session s1
    const staleCallback = fetchAndOpenFileMock.mock.calls[0]?.[2] as (file: OpenFileTab) => void;
    act(() => staleCallback(MOCK_FILE));

    // Should still be null because the callback belongs to the old session
    expect(result.current.selectedFile).toBeNull();
  });

  it("latest handleOpenFile call wins", () => {
    const { result } = renderHandlers();
    act(() => {
      result.current.handleOpenFile(MOCK_FILE);
      result.current.handleOpenFile(OTHER_FILE);
    });
    expect(result.current.selectedFile).toEqual(OTHER_FILE);
  });
});

describe("useMobilePanelHandlers request cancellation", () => {
  beforeEach(() => {
    fetchAndOpenFileMock.mockReset();
  });

  it("aborts stale chat file requests when a newer one starts", () => {
    const { result } = renderHandlers();
    act(() => result.current.handleOpenFileFromChat(CHAT_LINK_PATH));
    const firstOptions = fetchAndOpenFileMock.mock.calls[0]?.[4] as { signal: AbortSignal };

    act(() => result.current.handleOpenFileFromChat("src/newer.ts"));
    const secondOptions = fetchAndOpenFileMock.mock.calls[1]?.[4] as { signal: AbortSignal };

    expect(firstOptions.signal.aborted).toBe(true);
    expect(secondOptions.signal.aborted).toBe(false);
  });

  it("aborts stale chat file requests when the session changes", () => {
    const { result, rerender } = renderHandlers();
    act(() => result.current.handleOpenFileFromChat(CHAT_LINK_PATH));
    const firstOptions = fetchAndOpenFileMock.mock.calls[0]?.[4] as { signal: AbortSignal };

    rerender({ sid: "s2" });

    expect(firstOptions.signal.aborted).toBe(true);
  });
});

describe("useMobileReviewPanelFallback", () => {
  it("persists chat when the last linked review disappears", () => {
    const handlePanelChange = vi.fn();

    const { result } = renderHook(() =>
      useMobileReviewPanelFallback("review", false, handlePanelChange),
    );

    expect(result.current).toBe("chat");
    expect(handlePanelChange).toHaveBeenCalledOnce();
    expect(handlePanelChange).toHaveBeenCalledWith("chat");
  });

  it("keeps Review selected while a registered provider is still loading", () => {
    const handlePanelChange = vi.fn();
    const { result } = renderHook(() =>
      useMobileReviewPanelFallback("review", false, handlePanelChange, true),
    );

    expect(result.current).toBe("review");
    expect(handlePanelChange).not.toHaveBeenCalled();
  });
});

describe("MobilePanelArea PR identity", () => {
  it("remounts detail feedback when the user chooses another mixed-provider review", () => {
    function MobileReviewHarness() {
      const reviews: ReviewItemSummary[] = [
        {
          providerId: "github",
          reviewKey: "pr-a",
          title: "GitHub pull request",
          url: "https://github.test/a",
          repositoryId: "owner/repository",
          state: "OPEN",
        },
        {
          providerId: "bitbucket",
          reviewKey: "pr-b",
          title: "Bitbucket pull request",
          url: "https://bitbucket.test/b",
          repositoryId: "workspace/repository",
          state: "OPEN",
        },
      ];
      const [selectedReview, setSelectedReview] = useState<ReviewItemSummary | null>(reviews[0]!);
      return (
        <MobilePanelArea
          currentMobilePanel="review"
          activeTaskId="task-1"
          isPassthroughMode={false}
          effectiveSessionId="session-1"
          selectedFile={null}
          selectedFilePreview={false}
          selectedDiff={null}
          handleOpenFileFromChat={vi.fn()}
          handleClearSelectedDiff={vi.fn()}
          handleOpenFile={vi.fn()}
          handlePanelChangeAndClearSheet={vi.fn()}
          topNavHeight="3.5rem"
          bottomNavHeight="3.25rem"
          reviews={reviews}
          selectedReview={selectedReview}
          onSelectReview={setSelectedReview}
        />
      );
    }

    render(<MobileReviewHarness />);
    expect(screen.getByRole("button", { name: "feedback for pr-a" })).not.toBeNull();

    act(() => screen.getByRole("button", { name: "Select review B" }).click());

    expect(screen.queryByRole("button", { name: "feedback for pr-a" })).toBeNull();
    expect(screen.getByRole("button", { name: "feedback for pr-b" })).not.toBeNull();
  });
});
