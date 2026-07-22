import { forwardRef, type HTMLAttributes } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";

const navigation = vi.hoisted(() => ({
  goPrevious: vi.fn(),
  goNext: vi.fn(),
  options: null as null | { navigateTo: (id: string) => boolean | Promise<boolean> },
}));

vi.mock("@/hooks/use-message-navigation", () => ({
  useUserMessageNavigation: (options: {
    navigateTo: (id: string) => boolean | Promise<boolean>;
  }) => {
    navigation.options = options;
    return {
      userMessageIds: ["user-1"],
      canNavigatePrevious: vi.fn(() => true),
      canNavigateNext: vi.fn(() => false),
      isBusy: false,
      goPrevious: navigation.goPrevious,
      goNext: navigation.goNext,
    };
  },
}));
vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => ({ loadMore: vi.fn(async () => 0), hasMore: true, isLoading: false }),
}));
vi.mock("@kandev/ui/pannel-session", () => ({
  SessionPanelContent: forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(
    ({ children, ...props }, ref) => (
      <div ref={ref} data-testid="native-scroll-owner" {...props}>
        {children}
      </div>
    ),
  ),
}));
vi.mock("@/components/task/chat/messages/agent-status", () => ({ AgentStatus: () => null }));
vi.mock("@/components/task/chat/message-renderer", () => ({
  MessageRenderer: () => <div>prompt</div>,
}));
vi.mock("@/components/task/chat/messages/turn-group-message", () => ({
  TurnGroupMessage: () => null,
}));
vi.mock("@/components/session/prepare-progress", () => ({ PrepareProgress: () => null }));

class IntersectionObserverStub {
  observe() {}
  disconnect() {}
}
vi.stubGlobal("IntersectionObserver", IntersectionObserverStub);
afterEach(cleanup);

import { NativeMessageList } from "./message-list-native";

function props() {
  const message = { id: "user-1", author_type: "user", type: "message" } as Message;
  return {
    items: [{ type: "message", message }] as RenderItem[],
    messages: [message],
    permissionsByToolCallId: new Map<string, Message>(),
    childrenByParentToolCallId: new Map<string, Message[]>(),
    sessionId: "session-1",
    messagesLoading: false,
    isWorking: false,
  };
}

describe("NativeMessageList user navigation", () => {
  it("does not mount the former floating rail or reserve its content clearance", () => {
    const { container } = render(<NativeMessageList {...props()} />);

    expect(screen.queryByTestId("user-message-navigation-rail")).toBeNull();
    expect(screen.getByTestId("native-scroll-owner").className).not.toContain(
      "safe-area-inset-right",
    );
    expect(container.querySelector('[data-testid="load-older-messages"]')).not.toBeNull();
  });

  it("centers the destination DOM prompt and replays its highlight", async () => {
    vi.useFakeTimers();
    const { container } = render(<NativeMessageList {...props()} />);
    const destination = container.querySelector('[data-user-message-id="user-1"]') as HTMLElement;
    const scrollIntoView = vi.fn();
    destination.scrollIntoView = scrollIntoView;

    await act(async () => {
      const pending = navigation.options?.navigateTo("user-1");
      await vi.advanceTimersByTimeAsync(300);
      await pending;
    });

    expect(scrollIntoView).toHaveBeenCalledWith({ block: "center", behavior: "smooth" });
    expect(destination.classList.contains("search-flash")).toBe(true);
    vi.useRealTimers();
  });
});
