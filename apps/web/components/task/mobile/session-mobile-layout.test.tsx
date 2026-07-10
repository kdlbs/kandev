import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { OpenFileTab } from "@/lib/types/backend";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const fetchAndOpenFileMock = vi.fn();
vi.mock("../file-browser-hooks", () => ({
  fetchAndOpenFile: (...args: unknown[]) => fetchAndOpenFileMock(...args),
}));

import { useMobilePanelHandlers } from "./session-mobile-layout";

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
});
