import { forwardRef, type HTMLAttributes } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";
import { USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS } from "./user-message-navigation-rail";

const navigation = vi.hoisted(() => ({
  goPrevious: vi.fn(),
  goNext: vi.fn(),
  options: null as null | { navigateTo: (id: string) => boolean | Promise<boolean> },
}));
const breakpoint = vi.hoisted(() => ({ isFinePointer: false, isMobile: true }));

vi.mock("@/hooks/use-message-navigation", () => ({
  useUserMessageNavigation: (options: {
    navigateTo: (id: string) => boolean | Promise<boolean>;
  }) => {
    navigation.options = options;
    return {
      userMessageIds: ["user-1"],
      originId: "user-1",
      setViewportOrigin: vi.fn(),
      hasPrevious: true,
      hasNext: false,
      isBusy: false,
      goPrevious: navigation.goPrevious,
      goNext: navigation.goNext,
    };
  },
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpoint,
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
  beforeEach(() => {
    breakpoint.isFinePointer = false;
    breakpoint.isMobile = true;
  });
  it("mounts the rail outside the scroll owner and reserves mobile content clearance", () => {
    const { container } = render(<NativeMessageList {...props()} />);

    const viewport = screen.getByTestId("user-message-navigation-rail").parentElement;
    expect(viewport?.className).toContain("group/chat");
    expect(viewport?.querySelector('[data-testid="native-scroll-owner"] nav')).toBeNull();
    expect(screen.getByTestId("native-scroll-owner").className).toContain(
      USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS,
    );
    expect(container.querySelector('[data-testid="load-older-messages"]')).not.toBeNull();
  });

  it("does not reserve rail clearance for a fine-pointer desktop", () => {
    breakpoint.isFinePointer = true;
    breakpoint.isMobile = false;
    render(<NativeMessageList {...props()} />);

    expect(screen.getByTestId("native-scroll-owner").className).not.toContain(
      USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS,
    );
  });

  it("centers the destination DOM prompt and replays its highlight", async () => {
    const { container } = render(<NativeMessageList {...props()} />);
    const destination = container.querySelector('[data-user-message-id="user-1"]') as HTMLElement;
    const scrollIntoView = vi.fn();
    destination.scrollIntoView = scrollIntoView;

    await act(async () => navigation.options?.navigateTo("user-1"));

    expect(scrollIntoView).toHaveBeenCalledWith({ block: "center", behavior: "smooth" });
    expect(destination.classList.contains("search-flash")).toBe(true);
  });
});
