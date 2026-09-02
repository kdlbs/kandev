/* eslint-disable max-lines -- these tests cover the complete mobile session composition. */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, renderHook, act, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import type { OpenFileTab } from "@/lib/types/backend";
import type { ReviewItemSummary } from "@/lib/plugins/types";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

const fetchAndOpenFileMock = vi.fn();
const getWebSocketClientMock = vi.fn();
const requestFileContentMock = vi.fn();
const websocketClientMock = {
  on: vi.fn<(type: string, handler: (message: unknown) => void) => () => void>(
    () => () => undefined,
  ),
};
vi.mock("../file-browser-hooks", () => ({
  fetchAndOpenFile: (...args: unknown[]) => fetchAndOpenFileMock(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClientMock(),
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => requestFileContentMock(...args),
}));

vi.mock("../task-plan-panel", () => ({
  TaskPlanPanel: (props: { taskId: string | null; mobileBottomOffset?: string }) => (
    <div
      data-testid="mock-task-plan-panel"
      data-task-id={props.taskId ?? ""}
      data-mobile-bottom-offset={props.mobileBottomOffset}
    />
  ),
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

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1", activeSessionId: "session-1" },
      connection: { status: "connected" },
    }),
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

vi.mock("../prompt-history-panel-content", () => ({
  PromptHistoryPanelContent: ({
    onNavigateToPrompt,
  }: {
    onNavigateToPrompt?: (messageId: string) => void;
  }) => (
    <button
      type="button"
      data-testid="mobile-prompt-history-content"
      onClick={() => onNavigateToPrompt?.("prompt-1")}
    >
      Prompt history
    </button>
  ),
}));

import {
  MobilePanelArea,
  resolveMobilePluginPanel,
  resolveMobileReviewSource,
  terminalPaddingBottom,
  useMobilePanelHandlers,
  useMobileReviewPanelFallback,
} from "./session-mobile-layout";
import type { PluginLifecycleSnapshot } from "@/lib/plugins/registry";
import { pluginRegistry } from "@/lib/plugins/registry";

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
const DIRTY_DRAFT_CONTENT = "draft content";

function renderHandlers(initialSid: string | null = "s1") {
  const handlePanelChange = vi.fn();
  const view = renderHook(
    ({ sid }) => useMobilePanelHandlers({ effectiveSessionId: sid, handlePanelChange }),
    { initialProps: { sid: initialSid } },
  );
  return { handlePanelChange, ...view };
}

// eslint-disable-next-line max-lines-per-function -- these tests cover the mobile file selection lifecycle.
describe("useMobilePanelHandlers", () => {
  beforeEach(() => {
    fetchAndOpenFileMock.mockReset();
    getWebSocketClientMock.mockReset();
    getWebSocketClientMock.mockReturnValue(websocketClientMock);
    websocketClientMock.on.mockReset();
    websocketClientMock.on.mockImplementation(() => () => undefined);
    requestFileContentMock.mockReset();
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
    expect(result.current.selectedFileMode).toBe("preview");
    expect(handlePanelChange).toHaveBeenCalledWith("files");
  });

  it("opens a Markdown file from the Files panel in Preview mode", () => {
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({ ...MOCK_FILE, path: "README.md", name: "README.md" }),
    );

    expect(result.current.selectedFileMode).toBe("preview");
    expect(result.current.selectedFile?.markdownMode).toBe("preview");
  });

  it("updates the selected mobile buffer and mode without changing file identity", () => {
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({ ...MOCK_FILE, path: "README.md", name: "README.md" }),
    );

    act(() => result.current.handleSelectedFileModeChange("edit"));
    act(() => result.current.handleSelectedFileChange("# changed"));

    expect(result.current.selectedFile).toMatchObject({
      path: "README.md",
      content: "# changed",
      isDirty: true,
      markdownMode: "edit",
    });
  });

  it("keeps newer mobile edits when an older save completes", () => {
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({
        ...MOCK_FILE,
        path: "README.md",
        name: "README.md",
        content: "# base",
        originalContent: "# base",
      }),
    );

    act(() => result.current.handleSelectedFileChange("# first"));
    act(() =>
      result.current.handleSelectedFileSaved({
        path: "README.md",
        sessionId: "s1",
        content: "# first",
        originalContent: "# first",
        originalHash: "first-hash",
      }),
    );
    act(() => result.current.handleSelectedFileChange("# second"));
    act(() =>
      result.current.handleSelectedFileSaved({
        path: "README.md",
        sessionId: "s1",
        content: "# first",
        originalContent: "# first",
        originalHash: "first-hash",
      }),
    );

    expect(result.current.selectedFile).toMatchObject({
      content: "# second",
      originalContent: "# first",
      originalHash: "first-hash",
      isDirty: true,
    });
  });

  it("ignores a completed mobile save for another selected file", () => {
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({ ...MOCK_FILE, path: "README.md", name: "README.md" }),
    );
    act(() =>
      result.current.handleSelectedFileSaved({
        path: "other.md",
        sessionId: "s1",
        content: "# other",
        originalContent: "# other",
        originalHash: "other-hash",
      }),
    );

    expect(result.current.selectedFile).toEqual({
      ...MOCK_FILE,
      path: "README.md",
      name: "README.md",
      markdownMode: "preview",
    });
  });

  it("reloads a remote Markdown update into the clean mobile buffer", async () => {
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({
        ...MOCK_FILE,
        path: "README.md",
        name: "README.md",
        content: "# local",
        originalContent: "# base",
        isDirty: true,
        hasRemoteUpdate: true,
        remoteContent: "# remote",
        remoteOriginalHash: "remote-hash",
      }),
    );

    await act(async () => result.current.handleSelectedFileReload());

    expect(result.current.selectedFile).toMatchObject({
      path: "README.md",
      content: "# remote",
      originalContent: "# remote",
      originalHash: "remote-hash",
      isDirty: false,
      hasRemoteUpdate: false,
      remoteContent: undefined,
      remoteOriginalHash: undefined,
    });
  });

  it("refreshes a clean selected file after a workspace notification", async () => {
    requestFileContentMock.mockResolvedValue({ content: "# remote" });
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({
        ...MOCK_FILE,
        path: "README.md",
        name: "README.md",
        content: "# local",
        originalContent: "# local",
      }),
    );

    const handler = websocketClientMock.on.mock.calls[0]?.[1] as (message: unknown) => void;
    act(() => {
      handler({
        payload: {
          session_id: "s1",
          changes: [{ path: "README.md", operation: "write" }],
        },
      });
    });
    expect(requestFileContentMock).toHaveBeenCalledWith(
      websocketClientMock,
      "s1",
      "README.md",
      undefined,
    );
    await waitFor(() => expect(result.current.selectedFile?.content).toBe("# remote"));

    expect(result.current.selectedFile).toMatchObject({
      content: "# remote",
      originalContent: "# remote",
      isDirty: false,
      hasRemoteUpdate: false,
    });
  });

  it("preserves a dirty selected file and exposes a workspace reload", async () => {
    requestFileContentMock.mockResolvedValue({ content: "# remote" });
    const { result } = renderHandlers();
    act(() =>
      result.current.handleOpenFile({
        ...MOCK_FILE,
        path: "README.md",
        name: "README.md",
        content: "# local",
        originalContent: "# base",
        isDirty: true,
      }),
    );

    const handler = websocketClientMock.on.mock.calls[0]?.[1] as (message: unknown) => void;
    act(() => {
      handler({
        payload: {
          session_id: "s1",
          changes: [{ path: "README.md", operation: "write" }],
        },
      });
    });
    await waitFor(() => expect(result.current.selectedFile?.remoteContent).toBe("# remote"));

    expect(result.current.selectedFile).toMatchObject({
      content: "# local",
      originalContent: "# base",
      isDirty: true,
      hasRemoteUpdate: true,
      remoteContent: "# remote",
    });
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

  it("keeps a dirty viewer until the shared navigation discard is confirmed", () => {
    const { result, handlePanelChange } = renderHandlers();
    act(() => result.current.handleOpenFile(MOCK_FILE));
    act(() => result.current.handleSelectedFileChange(DIRTY_DRAFT_CONTENT));
    handlePanelChange.mockClear();

    act(() => result.current.handlePanelChangeAndClearSheet("plan"));

    expect(result.current.selectedFile).toMatchObject({
      path: MOCK_FILE.path,
      content: DIRTY_DRAFT_CONTENT,
      isDirty: true,
    });
    expect(result.current.pendingNavigation).toMatchObject({
      panel: "plan",
      filePath: MOCK_FILE.path,
    });
    expect(handlePanelChange).not.toHaveBeenCalled();

    act(() => result.current.confirmPendingNavigation());

    expect(result.current.selectedFile).toBeNull();
    expect(result.current.pendingNavigation).toBeNull();
    expect(handlePanelChange).toHaveBeenCalledWith("plan");
  });

  it("keeps the dirty viewer and cancels a pending navigation", () => {
    const { result, handlePanelChange } = renderHandlers();
    act(() => result.current.handleOpenFile(MOCK_FILE));
    act(() => result.current.handleSelectedFileChange(DIRTY_DRAFT_CONTENT));
    handlePanelChange.mockClear();

    act(() => result.current.handlePanelChangeAndClearSheet("files"));
    act(() => result.current.cancelPendingNavigation());

    expect(result.current.selectedFile).toMatchObject({
      path: MOCK_FILE.path,
      content: DIRTY_DRAFT_CONTENT,
      isDirty: true,
    });
    expect(result.current.pendingNavigation).toBeNull();
    expect(handlePanelChange).not.toHaveBeenCalled();
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

describe("useMobilePanelHandlers switcher navigation", () => {
  it("guards task and workspace switcher actions until a dirty viewer is confirmed", () => {
    const { result } = renderHandlers();
    const switcherAction = vi.fn();

    act(() => result.current.handleOpenFile(MOCK_FILE));
    act(() => result.current.handleSelectedFileChange(DIRTY_DRAFT_CONTENT));
    act(() => result.current.requestNavigation(switcherAction));

    expect(result.current.pendingNavigation).toMatchObject({
      panel: "files",
      filePath: MOCK_FILE.path,
    });
    expect(switcherAction).not.toHaveBeenCalled();

    act(() => result.current.cancelPendingNavigation());
    expect(switcherAction).not.toHaveBeenCalled();
    expect(result.current.selectedFile?.content).toBe(DIRTY_DRAFT_CONTENT);

    act(() => result.current.requestNavigation(switcherAction));
    act(() => result.current.confirmPendingNavigation());

    expect(switcherAction).toHaveBeenCalledOnce();
  });

  it("does not run a deferred switcher action after the selected file identity changes", () => {
    const { result } = renderHandlers();
    const switcherAction = vi.fn();

    act(() => result.current.handleOpenFile(MOCK_FILE));
    act(() => result.current.handleSelectedFileChange(DIRTY_DRAFT_CONTENT));
    act(() => result.current.requestNavigation(switcherAction));
    act(() => result.current.handleOpenFile(OTHER_FILE));
    act(() => result.current.confirmPendingNavigation());

    expect(switcherAction).not.toHaveBeenCalled();
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

describe("resolveMobilePluginPanel", () => {
  const panel = "plugin:plugin-a:notes" as const;

  function lifecycle(status: PluginLifecycleSnapshot["status"]): PluginLifecycleSnapshot {
    return { status, generation: 3 };
  }

  it("preserves the selected panel while its plugin is loading or recovering", () => {
    expect(resolveMobilePluginPanel(panel, lifecycle("loading"), false)).toBe(panel);
    expect(resolveMobilePluginPanel(panel, lifecycle("failed"), false)).toBe(panel);
  });

  it("falls back to Chat only after definitive removal or a ready missing registration", () => {
    expect(resolveMobilePluginPanel(panel, lifecycle("removed"), false)).toBe("chat");
    expect(resolveMobilePluginPanel(panel, lifecycle("ready"), false)).toBe("chat");
  });

  it("keeps a ready selected panel when its registration is present", () => {
    expect(resolveMobilePluginPanel(panel, lifecycle("ready"), true)).toBe(panel);
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
          connectionScope: "https://github.test",
          repositoryId: "owner/repository",
          changeRequestNumber: 1,
          state: "OPEN",
        },
        {
          providerId: "bitbucket",
          reviewKey: "pr-b",
          title: "Bitbucket pull request",
          url: "https://bitbucket.test/b",
          connectionScope: "https://bitbucket.test",
          repositoryId: "workspace/repository",
          changeRequestNumber: 2,
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
          selectedFileMode={undefined}
          selectedDiff={null}
          handleOpenFileFromChat={vi.fn()}
          handleClearSelectedDiff={vi.fn()}
          handleOpenFile={vi.fn()}
          handlePanelChangeAndClearSheet={vi.fn()}
          onNavigateToPrompt={vi.fn()}
          mobileScrollTarget={null}
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

describe("MobilePanelArea Prompt history", () => {
  it("renders the history surface and forwards prompt navigation", () => {
    const handleNavigateToPrompt = vi.fn();

    render(
      <MobilePanelArea
        currentMobilePanel="prompt-history"
        activeTaskId="task-1"
        isPassthroughMode={false}
        effectiveSessionId="session-1"
        selectedFile={null}
        selectedFileMode={undefined}
        selectedDiff={null}
        handleOpenFileFromChat={vi.fn()}
        handleClearSelectedDiff={vi.fn()}
        handleOpenFile={vi.fn()}
        handlePanelChangeAndClearSheet={vi.fn()}
        onNavigateToPrompt={handleNavigateToPrompt}
        mobileScrollTarget={null}
        topNavHeight="3.5rem"
        bottomNavHeight="3.25rem"
        reviews={[]}
        selectedReview={null}
        onSelectReview={vi.fn()}
      />,
    );

    expect(screen.getByTestId("mobile-prompt-history-content")).toBeTruthy();
    fireEvent.click(screen.getByTestId("mobile-prompt-history-content"));
    expect(handleNavigateToPrompt).toHaveBeenCalledWith("prompt-1");
  });
});

describe("MobilePanelArea Plan formatting offset", () => {
  it("passes the bottom navigation height to the Plan panel", () => {
    render(
      <MobilePanelArea
        currentMobilePanel="plan"
        activeTaskId="task-1"
        isPassthroughMode={false}
        effectiveSessionId="session-1"
        selectedFile={null}
        selectedFileMode={undefined}
        selectedDiff={null}
        handleOpenFileFromChat={vi.fn()}
        handleClearSelectedDiff={vi.fn()}
        handleOpenFile={vi.fn()}
        handlePanelChangeAndClearSheet={vi.fn()}
        onNavigateToPrompt={vi.fn()}
        mobileScrollTarget={null}
        topNavHeight="3.5rem"
        bottomNavHeight="3.25rem"
        reviews={[]}
        selectedReview={null}
        onSelectReview={vi.fn()}
      />,
    );

    expect(screen.getByTestId("mock-task-plan-panel").getAttribute("data-task-id")).toBe("task-1");
    expect(
      screen.getByTestId("mock-task-plan-panel").getAttribute("data-mobile-bottom-offset"),
    ).toBe("3.25rem");
  });
});

function renderMobilePanel(currentMobilePanel: string) {
  return render(
    <MobilePanelArea
      currentMobilePanel={currentMobilePanel as never}
      activeTaskId="task-1"
      isPassthroughMode={false}
      effectiveSessionId="session-1"
      selectedFile={null}
      selectedFileMode={undefined}
      selectedDiff={null}
      handleOpenFileFromChat={vi.fn()}
      handleClearSelectedDiff={vi.fn()}
      handleOpenFile={vi.fn()}
      handlePanelChangeAndClearSheet={vi.fn()}
      onNavigateToPrompt={vi.fn()}
      mobileScrollTarget={null}
      topNavHeight="3.5rem"
      bottomNavHeight="3.25rem"
      reviews={[]}
      selectedReview={null}
      onSelectReview={vi.fn()}
    />,
  );
}

describe("MobilePanelArea — plugin task panel (AC7)", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin("kandev-plugin-notes");
  });

  it("renders the plugin's Component with mobile presentation when selected", () => {
    function Notes(props: { panelId: string; taskId: string; presentation: string }) {
      return (
        <div data-testid="notes-mobile-body">
          {props.panelId}|{props.taskId}|{props.presentation}
        </div>
      );
    }
    pluginRegistry
      .forPlugin("kandev-plugin-notes")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });

    renderMobilePanel("plugin:kandev-plugin-notes:notes");

    expect(screen.getByTestId("notes-mobile-body").textContent).toBe(
      "plugin:kandev-plugin-notes:notes|task-1|mobile",
    );
  });

  it("renders nothing for a panel id that isn't a plugin id", () => {
    renderMobilePanel("not-a-plugin-panel-id");
    expect(screen.queryByTestId("notes-mobile-body")).toBeNull();
  });
});

describe("terminalPaddingBottom", () => {
  it("pads by the keybar height alone when the keyboard is closed", () => {
    expect(terminalPaddingBottom(false, 0, "3.25rem")).toBe("48px");
    // bottomOffset is irrelevant while the keyboard is closed.
    expect(terminalPaddingBottom(false, 300, "3.25rem")).toBe("48px");
  });

  it("subtracts the bottom nav and adds the live keyboard offset when the keyboard is open", () => {
    expect(terminalPaddingBottom(true, 300, "3.25rem")).toBe(
      "calc(348px - 3.25rem - env(safe-area-inset-bottom, 0px))",
    );
  });
});

describe("resolveMobileReviewSource", () => {
  it("keeps built-in source precedence for saved mobile state", () => {
    expect(resolveMobileReviewSource(true, true)).toBe("github");
    expect(resolveMobileReviewSource(false, true)).toBe("gitlab");
    expect(resolveMobileReviewSource(false, false)).toBeNull();
  });
});
