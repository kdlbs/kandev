import { useState } from "react";
import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";

const rendererSpy = vi.fn();
const mockStoreState = vi.hoisted(() => ({
  taskSessions: {
    items: {
      s1: {
        metadata: {
          last_agent_error: {
            message: "agent process exited",
            occurred_at: "2026-06-14T12:00:00Z",
          },
        },
      },
    },
  },
  dismissedAgentErrors: {} as Record<string, string>,
  dismissAgentError: () => {},
  setTaskSession: () => {},
}));

vi.mock("@/components/task/chat/message-renderer", () => ({
  MessageRenderer: (props: { onOpenFile?: unknown }) => {
    rendererSpy(props);
    return <div data-testid="renderer" />;
  },
}));
vi.mock("@/components/task/chat/messages/turn-group-message", () => ({
  TurnGroupMessage: () => <div data-testid="turn-group" />,
}));
vi.mock("@/components/session/prepare-progress", () => ({
  PrepareProgress: () => <div data-testid="prepare" />,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockStoreState) => unknown) => selector(mockStoreState),
  useAppStoreApi: () => ({
    getState: () => mockStoreState,
  }),
}));
vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => ({
    loadMore: async () => 0,
    hasMore: false,
    isLoading: false,
  }),
}));
vi.mock("@/components/task/chat/messages/agent-status", () => ({
  AgentStatus: () => <div data-testid="agent-status" />,
}));
vi.mock("@kandev/ui/pannel-session", () => ({
  SessionPanelContent: ({ children }: { children: ReactNode }) => (
    <div data-testid="session-panel-content">{children}</div>
  ),
}));
vi.mock("@/lib/api/domains/session-api", () => ({
  dismissLastAgentError: vi.fn(),
}));

import {
  MessageItem,
  MessageListStatus,
  getConversationLoadingState,
  getStreamingAgentMessageId,
} from "./message-list-shared";

const item: RenderItem = { type: "message", message: { id: "m1" } as Message };
const noop = () => {};
const perm = new Map<string, Message>();
const kids = new Map<string, Message[]>();

function row(onOpenFile: (p: string) => void) {
  return (
    <MessageItem
      item={item}
      sessionId="s1"
      permissionsByToolCallId={perm}
      childrenByParentToolCallId={kids}
      taskId="t1"
      worktreePath="/wt"
      onOpenFile={onOpenFile}
      isLastGroup={false}
      isTurnActive={false}
      onScrollToMessage={noop}
    />
  );
}

function Harness({ onOpenFile }: { onOpenFile: (p: string) => void }) {
  const [, setTick] = useState(0);
  return (
    <div>
      <button onClick={() => setTick((t) => t + 1)}>tick</button>
      {row(onOpenFile)}
    </div>
  );
}

describe("MessageItem memo boundary", () => {
  afterEach(() => {
    rendererSpy.mockClear();
  });

  it("does not re-render the row when the parent re-renders with stable props", () => {
    const { getByText } = render(<Harness onOpenFile={noop} />);
    expect(rendererSpy).toHaveBeenCalledTimes(1);
    fireEvent.click(getByText("tick"));
    fireEvent.click(getByText("tick"));
    expect(rendererSpy).toHaveBeenCalledTimes(1); // memo bailed on stable props
  });

  it("re-renders the row when onOpenFile identity changes (stability requirement)", () => {
    const { rerender } = render(row(() => {}));
    expect(rendererSpy).toHaveBeenCalledTimes(1);
    rerender(row(() => {}));
    expect(rendererSpy).toHaveBeenCalledTimes(2); // fresh callback ref breaks memo
  });
});

describe("MessageItem agent error notice", () => {
  it("shows retained agent errors even when there are no messages", () => {
    render(
      <MessageItem
        item={{
          type: "agent_error_notice",
          id: "last-agent-error-s1-2026-06-14T12:00:00Z",
          sessionId: "s1",
          error: {
            message: "agent process exited",
            occurredAt: "2026-06-14T12:00:00Z",
          },
        }}
        sessionId="s1"
        permissionsByToolCallId={perm}
        childrenByParentToolCallId={kids}
        taskId="t1"
        isLastGroup={false}
        isTurnActive={false}
        onScrollToMessage={noop}
      />,
    );

    expect(screen.getByTestId("last-agent-error-notice").getAttribute("role")).toBe("alert");
    expect(screen.queryByText("agent process exited")).not.toBeNull();
  });
});

describe("getConversationLoadingState", () => {
  it("shows loading while conversation history is still fetching with initial content rendered", () => {
    expect(
      getConversationLoadingState({
        messagesLoading: true,
        messagesCount: 1,
        isWorking: false,
        sessionState: "COMPLETED",
      }),
    ).toEqual({ isInitialLoading: false, showLoadingState: true });
  });

  it("shows an initial loading state when no messages are rendered yet", () => {
    expect(
      getConversationLoadingState({
        messagesLoading: true,
        messagesCount: 0,
        isWorking: false,
        sessionState: "RUNNING",
      }),
    ).toEqual({ isInitialLoading: true, showLoadingState: true });
  });

  it("does not compete with the active agent status while the session is working", () => {
    expect(
      getConversationLoadingState({
        messagesLoading: true,
        messagesCount: 1,
        isWorking: true,
        sessionState: "RUNNING",
      }),
    ).toEqual({ isInitialLoading: false, showLoadingState: false });
  });

  it("suppresses loading for empty sessions that cannot load conversation history", () => {
    expect(
      getConversationLoadingState({
        messagesLoading: true,
        messagesCount: 0,
        isWorking: false,
        sessionState: "FAILED",
      }),
    ).toEqual({ isInitialLoading: true, showLoadingState: false });

    expect(
      getConversationLoadingState({
        messagesLoading: true,
        messagesCount: 0,
        isWorking: false,
        sessionState: null,
      }),
    ).toEqual({ isInitialLoading: true, showLoadingState: false });
  });

  it("suppresses loading for CREATED sessions even when a synthetic task-description message is present", () => {
    // Prepare-only launches keep the session in CREATED with the "Start agent"
    // button as the primary CTA. useProcessedMessages injects a synthetic
    // task-description message so messagesCount is 1, but there is nothing to
    // load — the spinner would clash with the button.
    expect(
      getConversationLoadingState({
        messagesLoading: true,
        messagesCount: 1,
        isWorking: false,
        sessionState: "CREATED",
      }),
    ).toEqual({ isInitialLoading: false, showLoadingState: false });
  });
});

describe("getStreamingAgentMessageId", () => {
  it("only marks an agent reply after the latest user prompt as streaming", () => {
    const message = (id: string, author_type: "user" | "agent", type = "message") =>
      ({ id, author_type, type }) as Message;

    expect(
      getStreamingAgentMessageId([
        message("old-reply", "agent"),
        message("prompt", "user"),
        message("reply", "agent"),
        message("status", "agent", "status"),
      ]),
    ).toBe("reply");
    expect(getStreamingAgentMessageId([message("old-reply", "agent")])).toBeNull();
  });
});

describe("MessageListStatus", () => {
  it("renders a conversation loading indicator while existing content remains visible", () => {
    render(
      <MessageListStatus
        isLoadingMore={false}
        hasMore={false}
        showLoadingState
        messagesLoading
        isInitialLoading={false}
        messagesCount={1}
      />,
    );

    expect(screen.queryByTestId("conversation-loading-state")).not.toBeNull();
    expect(screen.queryByText("Loading conversation...")).not.toBeNull();
  });
});
