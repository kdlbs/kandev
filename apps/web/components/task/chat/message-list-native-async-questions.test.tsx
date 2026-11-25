import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";

const mocks = vi.hoisted(() => ({ asyncQuestionsEnabled: false }));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.asyncQuestionsEnabled,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ userSettings: { loaded: true, showTurnChangedFiles: false } }),
}));

vi.mock("@/hooks/use-chat-motion", () => ({ useChatMotion: () => false }));
vi.mock("@/hooks/use-lazy-load-messages", () => ({
  OLDER_PAGE_LIMIT: 50,
  useLazyLoadMessages: () => ({ loadMore: async () => 0, hasMore: false, isLoadingMore: false }),
}));
vi.mock("@/hooks/domains/session/use-session-turn", () => ({
  useSessionTurn: () => ({ activeTurnId: null }),
}));
vi.mock("@/hooks/domains/session/use-turn-changes", () => ({
  useSessionTurnChanges: () => ({ summaries: [], ensureTurnsLoaded: vi.fn() }),
}));
vi.mock("./use-transcript-auto-scroll-enabled", () => ({
  useTranscriptAutoScrollEnabled: () => false,
}));

vi.mock("@kandev/ui/pannel-session", async () => {
  const React = await import("react");
  return {
    SessionPanelContent: React.forwardRef<HTMLDivElement, ComponentProps<"div">>((props, ref) => (
      <div {...props} ref={ref} />
    )),
  };
});

vi.mock("./chat-motion", () => ({
  ChatMotionProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("./message-list-footer", () => ({ MessageListFooter: () => null }));

vi.mock("./message-list-native-scroll", () => ({
  findMessageRow: () => null,
  useNativeScrollManagement: () => ({
    handleScrollToMessage: vi.fn(),
    claimReaderPosition: vi.fn(),
    isReaderPositionClaimed: () => false,
    scrollToLatest: vi.fn(),
    sentinelRef: vi.fn(),
    markNotNearBottom: vi.fn(),
    isProgrammaticScrollLocked: () => false,
    retryLoadMore: vi.fn(),
    showRecovery: false,
  }),
}));

vi.mock("./transcript-auto-scroll", () => ({
  scheduleAfterPanelRestore: () => undefined,
  useActivationPending: (isVisible: boolean) => ({
    isVisibleRef: { current: isVisible },
    activationPendingRef: { current: false },
  }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(() => undefined, {
    getState: () => ({ pendingChatScrollTop: null, scrollTarget: null }),
  }),
}));

vi.mock("./message-list-shared", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./message-list-shared")>();
  return {
    ...actual,
    MessageListStatus: () => null,
    MessageItem: ({ item }: { item: { message: Message } }) => <span>{item.message.content}</span>,
    UnreadDivider: () => <div data-testid="unread-divider" />,
    getConversationLoadingState: () => ({ isInitialLoading: false, showLoadingState: false }),
    getEffectiveActiveTurnId: () => null,
    getLastTurnGroupId: () => null,
    getStreamingAgentMessageId: () => null,
    hasTranscriptContentBelowViewport: () => false,
    canReassertDividerScroll: () => false,
    resolveLastPromptEdge: () => "visible",
    findPromptNeighbors: () => null,
    resolveUnloadedPromptEdge: () => "visible",
    isElementFullyVisible: () => true,
  };
});

import { NativeMessageList } from "./message-list-native";

function message(id: string, type: Message["type"], content: string): Message {
  return {
    id,
    session_id: sessionId("session-1"),
    task_id: taskId("task-1"),
    author_type: "agent",
    type,
    content,
    created_at: "2026-10-30T10:00:00Z",
    ...(type === "agent_question_request" ? { metadata: { async_question_id: "q1" } } : {}),
  };
}

const retainedQuestion = message("retained-question", "agent_question_request", "Question");
const beforeQuestion = message("before-question", "message", "Before");
const afterQuestion = message("after-question", "message", "After");
const items: RenderItem[] = [beforeQuestion, retainedQuestion, afterQuestion].map((item) => ({
  type: "message",
  message: item,
}));

function renderTranscript() {
  return render(
    <NativeMessageList
      items={items}
      messages={[beforeQuestion, retainedQuestion, afterQuestion]}
      permissionsByToolCallId={new Map()}
      childrenByParentToolCallId={new Map()}
      taskId="task-1"
      sessionId="session-1"
      detailActive
      messagesLoading={false}
      isWorking={false}
      sessionState="RUNNING"
      dividerBeforeItemKey="retained-question"
    />,
  );
}

afterEach(() => {
  cleanup();
  mocks.asyncQuestionsEnabled = false;
  vi.unstubAllGlobals();
});

describe("NativeMessageList async question filtering", () => {
  it("removes retained question rows and anchors while off but keeps surrounding transcript", () => {
    mocks.asyncQuestionsEnabled = false;
    const view = renderTranscript();

    expect(view.container.querySelector("#msg-retained-question")).toBeNull();
    expect(view.container.querySelector("#msg-before-question")).not.toBeNull();
    expect(view.container.querySelector("#msg-after-question")).not.toBeNull();
    expect(
      view.container.querySelector("#msg-retained-question [data-testid='unread-divider']"),
    ).toBeNull();
  });

  it("keeps retained question rows when enabled", () => {
    mocks.asyncQuestionsEnabled = true;
    const view = renderTranscript();

    expect(view.container.querySelector("#msg-retained-question")).not.toBeNull();
    expect(screen.getByText("Question")).not.toBeNull();
  });
});
