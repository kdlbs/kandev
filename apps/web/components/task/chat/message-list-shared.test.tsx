/* eslint-disable max-lines -- shared renderer and transcript-navigation invariants use one mocked harness. */
import { useState } from "react";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  buildGroupedRenderItems,
  TASK_DESCRIPTION_SYNTHETIC_ID,
  type RenderItem,
} from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";

const rendererSpy = vi.fn();
const turnGroupSpy = vi.fn();

function elementWithRect(top: number, bottom: number): HTMLElement {
  const el = document.createElement("div");
  el.getBoundingClientRect = () => ({ top, bottom }) as DOMRect;
  return el;
}
const AGENT_ERROR_MESSAGE = "agent process exited";

// NOTE: mockStoreState is vi.hoisted, so it cannot reference the const above —
// the factory runs before module initialization.
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
  TurnGroupMessage: (props: unknown) => {
    turnGroupSpy(props);
    return <div data-testid="turn-group" />;
  },
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
    isLoadingMore: false,
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
  UnreadDivider,
  anchoredBarScrollOffsetPx,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getItemKey,
  getStreamingAgentMessageId,
  canReassertDividerScroll,
  getLastUserMessageId,
  getFirstUserMessageId,
  isElementFullyVisible,
  resolveLastPromptControls,
  resolveLastPromptEdge,
  shouldAutoScrollToBottom,
} from "./message-list-shared";

describe("anchoredBarScrollOffsetPx", () => {
  it("passes through a measured height unchanged", () => {
    expect(anchoredBarScrollOffsetPx(63)).toBe(63);
  });

  it("treats an unmeasured (undefined) height as no offset", () => {
    expect(anchoredBarScrollOffsetPx(undefined)).toBe(0);
  });

  it("clamps a negative height to zero", () => {
    expect(anchoredBarScrollOffsetPx(-12)).toBe(0);
  });

  it("rounds a fractional height to whole pixels", () => {
    expect(anchoredBarScrollOffsetPx(63.7)).toBe(64);
  });
});

const item: RenderItem = { type: "message", message: { id: "m1" } as Message };
const noop = () => {};
const perm = new Map<string, Message>();
const kids = new Map<string, Message[]>();

describe("getEffectiveActiveTurnId", () => {
  it("preserves the active turn while the session is working", () => {
    expect(getEffectiveActiveTurnId("turn-active", true)).toBe("turn-active");
  });

  it("ignores a stale active turn after the session settles", () => {
    expect(getEffectiveActiveTurnId("turn-stale", false)).toBeNull();
  });
});

describe("canReassertDividerScroll", () => {
  it("never applies without a divider target", () => {
    expect(
      canReassertDividerScroll({
        hasDividerTarget: false,
        didScrollToDivider: false,
        isUserScrolling: false,
        isWithinSettlingWindow: true,
      }),
    ).toBe(false);
  });

  it("always applies the first time, regardless of the other gates", () => {
    expect(
      canReassertDividerScroll({
        hasDividerTarget: true,
        didScrollToDivider: false,
        isUserScrolling: true,
        isWithinSettlingWindow: false,
      }),
    ).toBe(true);
  });

  it("re-asserts while still settling and the reader hasn't scrolled", () => {
    expect(
      canReassertDividerScroll({
        hasDividerTarget: true,
        didScrollToDivider: true,
        isUserScrolling: false,
        isWithinSettlingWindow: true,
      }),
    ).toBe(true);
  });

  it("stops re-asserting once the reader has scrolled, even mid-settling-window", () => {
    expect(
      canReassertDividerScroll({
        hasDividerTarget: true,
        didScrollToDivider: true,
        isUserScrolling: true,
        isWithinSettlingWindow: true,
      }),
    ).toBe(false);
  });

  it("stops re-asserting once the settling window elapses, even without any interaction", () => {
    // Covers a scrollbar drag or a live message arriving long after the
    // visit settled — neither fires wheel/touchstart/keydown, so the
    // settling window is the only thing bounding this case.
    expect(
      canReassertDividerScroll({
        hasDividerTarget: true,
        didScrollToDivider: true,
        isUserScrolling: false,
        isWithinSettlingWindow: false,
      }),
    ).toBe(false);
  });
});

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
      activeTurnId={null}
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
    turnGroupSpy.mockClear();
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
  afterEach(() => {
    cleanup();
  });

  const REMEDIATION_URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";

  function renderNotice(error: { message: string; occurredAt?: string; remediationUrl?: string }) {
    render(
      <MessageItem
        item={{
          type: "agent_error_notice",
          id: "last-agent-error-s1-2026-06-14T12:00:00Z",
          sessionId: "s1",
          error,
        }}
        sessionId="s1"
        permissionsByToolCallId={perm}
        childrenByParentToolCallId={kids}
        taskId="t1"
        isLastGroup={false}
        activeTurnId={null}
        onScrollToMessage={noop}
      />,
    );
  }

  it("shows retained agent errors even when there are no messages", () => {
    renderNotice({ message: AGENT_ERROR_MESSAGE });

    expect(screen.getByTestId("last-agent-error-notice").getAttribute("role")).toBe("alert");
    expect(screen.getByTestId("last-agent-error-notice").textContent).toContain(
      AGENT_ERROR_MESSAGE,
    );
  });

  it("renders a validated remediation link, and nothing for an invalid URL", () => {
    renderNotice({
      message: "usage limit reached",
      remediationUrl: REMEDIATION_URL,
    });

    const link = screen.getByTestId("remediation-link") as HTMLAnchorElement;
    expect(link.href).toBe(REMEDIATION_URL);
    expect(link.rel).toBe("noopener noreferrer");
    expect(screen.getByTestId("last-agent-error-notice").textContent).toContain(
      "usage limit reached",
    );

    cleanup();
    renderNotice({ message: "usage limit reached", remediationUrl: "https://evil.example.com/x" });
    expect(screen.queryByTestId("remediation-link")).toBeNull();
  });
});

describe("MessageItem containing turn activity", () => {
  afterEach(() => {
    rendererSpy.mockClear();
    turnGroupSpy.mockClear();
  });

  it("does not reactivate a historical message while another turn is active", () => {
    const historicalItem: RenderItem = {
      type: "message",
      message: { id: "historical", turn_id: "turn-old" } as Message,
    };
    render(
      <MessageItem
        item={historicalItem}
        sessionId="s1"
        permissionsByToolCallId={perm}
        childrenByParentToolCallId={kids}
        isLastGroup={false}
        activeTurnId="turn-new"
        onScrollToMessage={noop}
      />,
    );

    expect(rendererSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({ isContainingTurnActive: false }),
    );
  });

  it("marks only the matching turn group active", () => {
    const historicalGroup: RenderItem = {
      type: "turn_group",
      id: "group-old",
      turnId: "turn-old",
      messages: [],
    };
    const { rerender } = render(
      <MessageItem
        item={historicalGroup}
        sessionId="s1"
        permissionsByToolCallId={perm}
        childrenByParentToolCallId={kids}
        isLastGroup
        activeTurnId="turn-new"
        onScrollToMessage={noop}
      />,
    );
    expect(turnGroupSpy).toHaveBeenLastCalledWith(expect.objectContaining({ isTurnActive: false }));

    rerender(
      <MessageItem
        item={historicalGroup}
        sessionId="s1"
        permissionsByToolCallId={perm}
        childrenByParentToolCallId={kids}
        isLastGroup
        activeTurnId="turn-old"
        onScrollToMessage={noop}
      />,
    );
    expect(turnGroupSpy).toHaveBeenLastCalledWith(expect.objectContaining({ isTurnActive: true }));
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
    expect(getStreamingAgentMessageId([message("auto-started-reply", "agent")])).toBe(
      "auto-started-reply",
    );
  });
});

describe("getLastUserMessageId", () => {
  it("returns the id of the most recent user-authored message", () => {
    const message = (id: string, author_type: "user" | "agent") => ({ id, author_type }) as Message;

    expect(
      getLastUserMessageId([
        message("first-prompt", "user"),
        message("reply", "agent"),
        message("second-prompt", "user"),
        message("reply-2", "agent"),
      ]),
    ).toBe("second-prompt");
  });

  it("returns null when there are no user messages", () => {
    const message = (id: string, author_type: "user" | "agent") => ({ id, author_type }) as Message;

    expect(
      getLastUserMessageId([
        message(TASK_DESCRIPTION_SYNTHETIC_ID, "user"),
        message("reply", "agent"),
      ]),
    ).toBeNull();
    expect(getLastUserMessageId([])).toBeNull();
  });
});

describe("getFirstUserMessageId", () => {
  it("returns the id of the earliest user-authored message", () => {
    const message = (id: string, author_type: "user" | "agent") => ({ id, author_type }) as Message;

    expect(
      getFirstUserMessageId([
        message("agent-status", "agent"),
        message("first-prompt", "user"),
        message("reply", "agent"),
        message("second-prompt", "user"),
      ]),
    ).toBe("first-prompt");
  });

  it("returns null when there are no user messages", () => {
    const message = (id: string, author_type: "user" | "agent") => ({ id, author_type }) as Message;

    expect(
      getFirstUserMessageId([
        message(TASK_DESCRIPTION_SYNTHETIC_ID, "user"),
        message("reply", "agent"),
      ]),
    ).toBeNull();
    expect(getFirstUserMessageId([])).toBeNull();
  });
});

describe("shouldAutoScrollToBottom", () => {
  const base = {
    isNearBottom: true,
    isProgrammaticScrollLocked: false,
    hasPendingLayoutRestore: false,
  };

  it("forces the scroll when near bottom with nothing else claiming it", () => {
    expect(shouldAutoScrollToBottom(base)).toBe(true);
  });

  it("does not force the scroll while not near the bottom", () => {
    expect(shouldAutoScrollToBottom({ ...base, isNearBottom: false })).toBe(false);
  });

  it("does not force the scroll while a programmatic scroll-to-start/last-prompt is in flight", () => {
    // Regression: a message streaming in mid-flight must not silently
    // cancel a user-initiated scroll-to-start/scroll-to-last-prompt action.
    expect(shouldAutoScrollToBottom({ ...base, isProgrammaticScrollLocked: true })).toBe(false);
  });

  it("does not force the scroll while a layout-rebuild restore is pending", () => {
    expect(shouldAutoScrollToBottom({ ...base, hasPendingLayoutRestore: true })).toBe(false);
  });

  it("does not force the scroll when multiple claims are active at once", () => {
    expect(
      shouldAutoScrollToBottom({
        isNearBottom: true,
        isProgrammaticScrollLocked: true,
        hasPendingLayoutRestore: true,
      }),
    ).toBe(false);
  });
});

describe("prompt row identity survives turn-group merging", () => {
  // CodeRabbit review #3668338263 worried that `getItemKey` returns a
  // `turn_group` id for the row backing `lastPromptMessageId`/`firstMessageId`,
  // which would break the `msg-<id>` DOM lookup in
  // `useTranscriptEdgeTracking`. That can't happen: `groupActivityMessages`
  // only merges consecutive ACTIVITY_MESSAGE_TYPES messages (thinking/tool_*)
  // sharing a turn id, and user prompts are always emitted with
  // `type: "message"`, so they can never join a turn group. Lock in the
  // invariant so a future change to the activity type set can't silently
  // break the scroll affordances.
  function activityMessage(id: string, turnId: string): Message {
    return {
      id,
      session_id: "s1",
      task_id: "t1",
      author_type: "agent",
      content: "",
      type: "tool_call",
      turn_id: turnId,
      created_at: "",
    } as Message;
  }

  function promptMessage(id: string): Message {
    return {
      id,
      session_id: "s1",
      task_id: "t1",
      author_type: "user",
      content: "hi",
      type: "message",
      created_at: "",
    } as Message;
  }

  it("renders the first and last prompts as standalone message items, not turn_group members", () => {
    const firstPrompt = promptMessage("earliest-prompt");
    const activityA = activityMessage("think-1", "turn-1");
    const activityB = activityMessage("tool-1", "turn-1");
    const lastPrompt = promptMessage("latest-prompt");
    const allMessages = [firstPrompt, activityA, activityB, lastPrompt];

    const items = buildGroupedRenderItems(allMessages, "s1", { canAnchorPrepareProgress: false });

    const firstItem = items[0];
    const middleItem = items[1];
    const lastItem = items[items.length - 1];

    expect(firstItem.type).toBe("message");
    expect(middleItem.type).toBe("turn_group");
    expect(lastItem.type).toBe("message");

    expect(getItemKey(firstItem)).toBe(getFirstUserMessageId(allMessages));
    expect(getItemKey(lastItem)).toBe(getLastUserMessageId(allMessages));
    // The grouped activity's key is a synthetic turn-group id, never a raw message id.
    expect(getItemKey(middleItem)).not.toBe(activityA.id);
    expect(getItemKey(middleItem)).not.toBe(activityB.id);
  });
});

describe("resolveLastPromptEdge", () => {
  it("is 'above' once the target's bottom clears the two-pixel settle tolerance past the top", () => {
    const container = elementWithRect(40, 200);
    const target = elementWithRect(20, 35);

    expect(resolveLastPromptEdge(container, target)).toBe("above");
  });

  it("is 'visible' while any part of a prompt remains visible above the viewport", () => {
    const container = elementWithRect(40, 200);

    expect(resolveLastPromptEdge(container, elementWithRect(20, 80))).toBe("visible");
  });

  it("is 'below' once the target's top clears the tolerance past the bottom", () => {
    const container = elementWithRect(40, 200);

    expect(resolveLastPromptEdge(container, elementWithRect(220, 260))).toBe("below");
  });

  it("is 'visible' while the target is within the two-pixel settle tolerance", () => {
    const container = elementWithRect(40, 200);

    expect(resolveLastPromptEdge(container, elementWithRect(40, 80))).toBe("visible");
    expect(resolveLastPromptEdge(container, elementWithRect(38, 78))).toBe("visible");
  });
});

describe("resolveLastPromptControls", () => {
  it("shows the anchored bar and an upward scroll arrow only once the prompt is above the viewport", () => {
    expect(resolveLastPromptControls("above")).toEqual({
      anchoredBarVisible: true,
      scrollButtonEligible: true,
      scrollDirection: "up",
    });
  });

  it("hides the anchored bar and points the scroll arrow down while the prompt sits below the viewport", () => {
    expect(resolveLastPromptControls("below")).toEqual({
      anchoredBarVisible: false,
      scrollButtonEligible: true,
      scrollDirection: "down",
    });
  });

  it("hides both the anchored bar and the scroll button while the prompt is visible", () => {
    expect(resolveLastPromptControls("visible")).toEqual({
      anchoredBarVisible: false,
      scrollButtonEligible: false,
      scrollDirection: "up",
    });
  });
});

describe("isElementFullyVisible", () => {
  it("is true when the target sits entirely within the container's viewport", () => {
    const container = elementWithRect(0, 100);
    const target = elementWithRect(10, 50);
    expect(isElementFullyVisible(container, target)).toBe(true);
  });

  it("is false once the target's top has scrolled even slightly above the container's top", () => {
    const container = elementWithRect(40, 200);
    const target = elementWithRect(30, 80);
    expect(isElementFullyVisible(container, target)).toBe(false);
  });

  it("is false when the target's bottom overflows below the container's bottom", () => {
    const container = elementWithRect(0, 100);
    const target = elementWithRect(60, 150);
    expect(isElementFullyVisible(container, target)).toBe(false);
  });
});

describe("MessageListStatus", () => {
  it("does not render the older control during routine pagination", () => {
    render(
      <MessageListStatus
        isLoadingMore={false}
        hasMore
        showLoadingState={false}
        messagesLoading={false}
        isInitialLoading={false}
        messagesCount={1}
        onLoadMore={vi.fn()}
      />,
    );

    expect(screen.queryByTestId("load-older-messages")).toBeNull();
  });

  it("renders one explicit retry control during recovery", () => {
    const onLoadMore = vi.fn();
    render(
      <MessageListStatus
        isLoadingMore={false}
        hasMore
        showRecovery
        showLoadingState={false}
        messagesLoading={false}
        isInitialLoading={false}
        messagesCount={1}
        onLoadMore={onLoadMore}
      />,
    );

    const button = screen.getByTestId("load-older-messages");
    button.click();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

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

describe("UnreadDivider", () => {
  it("renders a labeled separator marking new messages", () => {
    render(<UnreadDivider />);

    const divider = screen.getByTestId("unread-divider");
    expect(divider.getAttribute("role")).toBe("separator");
    expect(screen.getByText("New")).not.toBeNull();
  });
});
