import { useImperativeHandle, type ReactNode, type Ref } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { MessageListHandle } from "./chat/message-list-shared";
import type { LoadMessageWindowResult } from "@/hooks/domains/session/load-message-window";

const { aroundRequest, scrollToMessage } = vi.hoisted(() => ({
  aroundRequest: vi.fn(),
  scrollToMessage: vi.fn((_messageId: string) => false),
}));
const SESSION = "prompt-panel-session";
const CONTROL_ID = "last-prompt-control";
const MESSAGE_LIST_ID = "message-list";
const prompt = {
  id: "persisted-prompt",
  session_id: SESSION,
  task_id: "task",
  author_type: "user",
  type: "message",
  content: "Persisted prompt",
  created_at: "2026-08-22T00:00:00Z",
} as Message;
const agent = {
  ...prompt,
  id: "newest-agent",
  author_type: "agent",
  content: "Agent output",
  created_at: "2026-08-22T00:00:01Z",
} as Message;
const panelState = {
  resolvedSessionId: SESSION,
  session: { state: "WAITING_FOR_INPUT", metadata: null },
  taskId: "task",
  isWorking: false,
  messagesLoading: false,
  historyRefreshPending: false,
  isInitialMessagesLoading: false,
  groupedItems: [{ type: "message", message: agent }] as RenderItem[],
  allMessages: [agent] as Message[],
  footerActionMessages: [] as Message[],
  permissionsByToolCallId: new Map(),
  childrenByParentToolCallId: new Map(),
  agentMessageCount: 1,
  pendingClarification: null,
  pendingClarificationGroup: null,
};
const state = {
  connection: { status: "disconnected" },
  kanbanMulti: { snapshots: {} },
  kanban: { workflowId: null, tasks: [], steps: [] },
  workflows: { items: [] },
  taskSessions: { items: { [SESSION]: { name: null, agent_profile_id: null } } },
  agentProfiles: { items: [] },
  userSettings: {
    showAnchoredPromptBar: true,
    showScrollToLastPrompt: true,
    showScrollToStart: false,
  },
  messages: {
    bySession: { [SESSION]: [agent] as Message[] },
    metaBySession: { [SESSION]: { hasMore: true, oldestCursor: "newest-agent" } },
  },
  messagePrompts: {
    bySession: { [SESSION]: [prompt] as Message[] },
    metaBySession: {},
    generationBySession: { [SESSION]: 0 },
    refreshGenerationBySession: {},
    observedBySession: {
      [SESSION]: {
        ids: { [prompt.id]: true },
        newestKey: { id: prompt.id, created_at: prompt.created_at } as {
          id: string;
          created_at: string;
        } | null,
      },
    },
    authoritativeBySession: { [SESSION]: true },
    deletedIdsBySession: {},
  },
  launchWarning: { bySessionId: {} },
  mergeMessages: vi.fn(),
  setPromptMessagesLoading: vi.fn(),
  installAuthoritativePromptMessages: vi.fn(),
};
const storeApi = { getState: () => state };
vi.mock("./panel-primitives", () => ({
  PanelRoot: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  PanelBody: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useOptionalAppStore: (selector: (value: typeof state) => unknown, fallback: unknown) =>
    selector(state) ?? fallback,
  useAppStoreApi: () => storeApi,
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: () => undefined }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));
vi.mock("./task-archived-context", () => ({ useIsTaskArchived: () => false }));
vi.mock("./task-launch-error-context", () => ({ useTaskLaunchErrorContext: () => null }));
vi.mock("@/hooks/domains/task/use-task-status-summary", () => ({
  useTaskStatusSummary: () => null,
}));
vi.mock("./chat/use-chat-panel-state", () => ({ useChatPanelState: () => panelState }));
vi.mock("./chat/chat-input-area", () => ({
  ChatInputArea: ({
    showScrollToLastPrompt,
    onScrollToLastPrompt,
  }: {
    showScrollToLastPrompt: boolean;
    onScrollToLastPrompt: () => void;
  }) =>
    showScrollToLastPrompt ? (
      <button data-testid="last-prompt-control" onClick={onScrollToLastPrompt}>
        prompt
      </button>
    ) : null,
  useSubmitHandler: () => ({ isSending: false, handleSubmit: vi.fn() }),
  useChatPanelHandlers: () => ({ handleCancelTurn: vi.fn() }),
}));
vi.mock("@/components/task/chat/message-list", () => ({
  MessageList: ({
    ref,
    lastPromptMessageId,
    anchoredBarHeight,
    stickyPromptBar,
  }: {
    ref?: Ref<MessageListHandle>;
    lastPromptMessageId?: string | null;
    anchoredBarHeight?: number;
    stickyPromptBar?: ReactNode;
  }) => {
    useImperativeHandle(ref, () => ({ scrollToMessage, scrollToLatest: vi.fn(() => true) }), []);
    return (
      <div
        data-testid={MESSAGE_LIST_ID}
        data-prompt-id={lastPromptMessageId ?? ""}
        data-bar-height={anchoredBarHeight ?? 0}
        data-sticky-prompt-bar={stickyPromptBar ? "present" : "absent"}
      />
    );
  },
}));
vi.mock("@/components/shared/task-markdown-file-link-provider", () => ({
  TaskMarkdownFileLinkProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./chat/clarification-panel-section", () => ({ ClarificationPanelSection: () => null }));
vi.mock("./chat/use-composer-agent-start-hint", () => ({ useComposerAgentStartHint: () => false }));
vi.mock("@/hooks/use-panel-search", () => ({ usePanelSearch: () => undefined }));
vi.mock("@/hooks/domains/session/use-session-search", () => ({
  useSessionSearch: () => ({
    isOpen: false,
    hits: [],
    activeHitId: null,
    open: vi.fn(),
    close: vi.fn(),
    setActiveHit: vi.fn(),
  }),
}));
vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => ({ loadMoreRaw: vi.fn(), hasMore: false }),
}));
vi.mock("@/components/task/chat/use-drain-older-messages", () => ({
  useDrainOlderMessages: () => undefined,
}));
vi.mock("./chat/use-session-read-tracking", () => ({ useSessionReadTracking: () => null }));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: { scrollTarget: null }) => unknown) => selector({ scrollTarget: null }),
    { getState: () => ({ scrollTarget: null }) },
  ),
}));
vi.mock("@/hooks/domains/session/load-message-window", () => ({
  loadMessageWindowAround: aroundRequest,
}));
vi.mock("@/hooks/domains/session/use-session-prompts", () => ({
  useSessionPrompts: () => ({ prompts: state.messagePrompts.bySession[SESSION] }),
}));
vi.mock("@/lib/session-workspace-path", () => ({ getSessionWorkspacePath: () => undefined }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { TaskChatPanel } from "./task-chat-panel";

beforeEach(() => {
  vi.clearAllMocks();
  scrollToMessage.mockReset().mockReturnValue(false);
  state.messages.bySession[SESSION] = [agent];
  state.messagePrompts.bySession[SESSION] = [prompt];
  state.messagePrompts.observedBySession[SESSION] = {
    ids: { [prompt.id]: true },
    newestKey: { id: prompt.id, created_at: prompt.created_at },
  };
  panelState.allMessages = [agent];
  panelState.groupedItems = [{ type: "message", message: agent }];
});
afterEach(() => cleanup());

describe("unloaded last prompt (AC-UI-PINNED-PROMPT-AVAILABILITY-001.1/.2/.6/.9)", () => {
  it("offers the control from an observed prompt without inserting a row or changing pagination", () => {
    render(<TaskChatPanel sessionId={SESSION} taskId="task" />);
    expect(screen.getByTestId(CONTROL_ID)).toBeTruthy();
    expect(screen.getByTestId(MESSAGE_LIST_ID).getAttribute("data-prompt-id")).toBe(prompt.id);
    expect(state.messages.bySession[SESSION]).toEqual([agent]);
    expect(state.messages.metaBySession[SESSION]).toMatchObject({
      hasMore: true,
      oldestCursor: "newest-agent",
    });
    expect(aroundRequest).not.toHaveBeenCalled();
  });

  it("withholds the control and anchored bar when the window is empty", () => {
    state.messages.bySession[SESSION] = [];
    render(<TaskChatPanel sessionId={SESSION} taskId="task" />);
    expect(screen.queryByTestId(CONTROL_ID)).toBeNull();
    expect(screen.getByTestId(MESSAGE_LIST_ID).getAttribute("data-sticky-prompt-bar")).toBe(
      "absent",
    );
    expect(screen.getByTestId(MESSAGE_LIST_ID).getAttribute("data-bar-height")).toBe("0");
  });

  it("does not mount or reserve the bar for a cached prompt filtered out of rendered rows", () => {
    state.messages.bySession[SESSION] = [prompt, agent];
    panelState.allMessages = [agent];
    panelState.groupedItems = [{ type: "message", message: agent }];
    render(<TaskChatPanel sessionId={SESSION} taskId="task" />);
    expect(screen.getByTestId(MESSAGE_LIST_ID).getAttribute("data-sticky-prompt-bar")).toBe(
      "absent",
    );
    expect(screen.getByTestId(MESSAGE_LIST_ID).getAttribute("data-bar-height")).toBe("0");
  });

  it("loads an unloaded prompt only after activation and shows the existing jump indication", async () => {
    aroundRequest.mockReturnValueOnce(Promise.withResolvers().promise);
    render(<TaskChatPanel sessionId={SESSION} taskId="task" />);
    fireEvent.click(screen.getByTestId(CONTROL_ID));
    await waitFor(() =>
      expect(aroundRequest).toHaveBeenCalledExactlyOnceWith(
        SESSION,
        prompt.id,
        expect.any(Function),
        expect.anything(),
      ),
    );
    expect(screen.getByTestId("transcript-jump-loading")).toBeTruthy();
    expect(scrollToMessage).toHaveBeenCalledWith(
      prompt.id,
      expect.objectContaining({ align: "start" }),
    );
  });

  it("lands the local jump when a merged raw row reaches the rendered transcript later", async () => {
    const pending = Promise.withResolvers<LoadMessageWindowResult>();
    aroundRequest.mockReturnValueOnce(pending.promise);
    let rowMounted = false;
    scrollToMessage.mockImplementation((id: string) => rowMounted && id === prompt.id);
    const view = render(<TaskChatPanel sessionId={SESSION} taskId="task" onOpenFile={vi.fn()} />);
    fireEvent.click(screen.getByTestId(CONTROL_ID));
    await waitFor(() => expect(aroundRequest).toHaveBeenCalledTimes(1));

    state.messages.bySession[SESSION] = [prompt, agent];
    panelState.allMessages = [prompt, agent];
    view.rerender(<TaskChatPanel sessionId={SESSION} taskId="task" onOpenFile={vi.fn()} />);
    await act(async () =>
      pending.resolve({
        kind: "merged",
        merged: true,
        current: true,
        targetFound: true,
      }),
    );
    const attemptsBeforeRow = scrollToMessage.mock.calls.length;

    rowMounted = true;
    panelState.groupedItems = [
      { type: "message", message: prompt },
      { type: "message", message: agent },
    ];
    view.rerender(<TaskChatPanel sessionId={SESSION} taskId="task" onOpenFile={vi.fn()} />);
    await waitFor(() =>
      expect(scrollToMessage.mock.calls.length).toBeGreaterThan(attemptsBeforeRow),
    );
    expect(scrollToMessage).toHaveBeenLastCalledWith(
      prompt.id,
      expect.objectContaining({ align: "start" }),
    );
  });

  it("scrolls a cached prompt immediately without starting an around request", () => {
    state.messages.bySession[SESSION] = [prompt, agent];
    render(<TaskChatPanel sessionId={SESSION} taskId="task" />);
    // The loaded control appears once edge tracking classifies the row out of view.
    expect(screen.queryByTestId(CONTROL_ID)).toBeNull();
    expect(aroundRequest).not.toHaveBeenCalled();
  });
});
