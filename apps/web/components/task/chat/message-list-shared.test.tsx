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

import { MessageItem } from "./message-list-shared";
import { VirtuosoMessageList } from "./message-list-virtuoso";

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

describe("VirtuosoMessageList empty state", () => {
  it("shows retained agent errors even when there are no messages", () => {
    render(
      <VirtuosoMessageList
        items={[]}
        messages={[]}
        permissionsByToolCallId={perm}
        childrenByParentToolCallId={kids}
        sessionId="s1"
        messagesLoading={false}
        isWorking={false}
        sessionState="WAITING_FOR_INPUT"
      />,
    );

    expect(screen.getByTestId("last-agent-error-notice").getAttribute("role")).toBe("alert");
    expect(screen.queryByText("agent process exited")).not.toBeNull();
  });
});
