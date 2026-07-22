import { forwardRef, useImperativeHandle, type HTMLAttributes } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";
import { waitForUserMessageElement } from "./message-list-shared";

const SCROLL_OWNER_TEST_ID = "virtuoso-scroll-owner";
const USER_ONE_ID = "user-1";
const USER_TWO_ID = "user-2";
const USER_ONE_SELECTOR = `[data-user-message-id="${USER_ONE_ID}"]`;
const SEARCH_FLASH_CLASS = "search-flash";

const navigation = vi.hoisted(() => ({
  options: null as null | { navigateTo: (id: string) => boolean | Promise<boolean> },
  scrollToIndex: vi.fn(),
  renderOffset: 0,
  isBusy: false,
  followOutput: undefined as false | ((isAtBottom: boolean) => "smooth" | false) | undefined,
  scrollOwners: [] as HTMLDivElement[],
}));
const breakpoint = vi.hoisted(() => ({ isFinePointer: false, isMobile: true }));

vi.mock("@/hooks/use-message-navigation", () => ({
  useUserMessageNavigation: (options: {
    navigateTo: (id: string) => boolean | Promise<boolean>;
  }) => {
    navigation.options = options;
    return {
      userMessageIds: [USER_ONE_ID, USER_TWO_ID],
      canNavigatePrevious: vi.fn(() => true),
      canNavigateNext: vi.fn(() => false),
      isBusy: navigation.isBusy,
      goPrevious: vi.fn(),
      goNext: vi.fn(),
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
      <div
        ref={(node) => {
          if (node) {
            Object.defineProperty(node, "offsetHeight", { configurable: true, value: 400 });
            if (!navigation.scrollOwners.includes(node)) navigation.scrollOwners.push(node);
          }
          if (typeof ref === "function") ref(node);
          else if (ref) ref.current = node;
        }}
        data-testid={SCROLL_OWNER_TEST_ID}
        {...props}
      >
        {children}
      </div>
    ),
  ),
}));
vi.mock("react-virtuoso", () => ({
  Virtuoso: forwardRef(
    (
      props: {
        firstItemIndex: number;
        itemContent: (index: number) => React.ReactNode;
        components: { Header: () => React.ReactNode };
        followOutput: false | ((isAtBottom: boolean) => "smooth" | false);
      },
      ref,
    ) => {
      useImperativeHandle(ref, () => ({ scrollToIndex: navigation.scrollToIndex }));
      navigation.followOutput = props.followOutput;
      return (
        <div data-testid="virtuoso-body">
          {props.components.Header()}
          {props.itemContent(props.firstItemIndex + navigation.renderOffset)}
        </div>
      );
    },
  ),
}));
vi.mock("@/components/task/chat/messages/agent-status", () => ({ AgentStatus: () => null }));
vi.mock("@/components/task/chat/message-renderer", () => ({ MessageRenderer: () => null }));
vi.mock("@/components/task/chat/messages/turn-group-message", () => ({
  TurnGroupMessage: () => null,
}));
vi.mock("@/components/session/prepare-progress", () => ({ PrepareProgress: () => null }));

import { VirtuosoMessageList } from "./message-list-virtuoso";

afterEach(cleanup);

function props() {
  const message = { id: USER_ONE_ID, author_type: "user", type: "message" } as Message;
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

beforeEach(() => {
  navigation.renderOffset = 0;
  navigation.isBusy = false;
  navigation.followOutput = undefined;
  navigation.scrollToIndex.mockReset();
  navigation.scrollOwners = [];
});

describe("VirtuosoMessageList structure", () => {
  it("keeps the same scroll owner when Virtuoso becomes ready", () => {
    render(<VirtuosoMessageList {...props()} />);

    expect(navigation.scrollOwners).toHaveLength(1);
    expect(screen.getByTestId(SCROLL_OWNER_TEST_ID)).toBe(navigation.scrollOwners[0]);
  });

  it("does not mount the former floating rail or reserve its content clearance", () => {
    render(<VirtuosoMessageList {...props()} />);

    expect(screen.queryByTestId("user-message-navigation-rail")).toBeNull();
    expect(screen.getByTestId(SCROLL_OWNER_TEST_ID).className).not.toContain(
      "safe-area-inset-right",
    );
    expect(screen.getByTestId("load-older-messages")).not.toBeNull();
  });

  it("suspends live bottom-follow while explicit navigation is busy", () => {
    navigation.isBusy = true;
    render(<VirtuosoMessageList {...props()} />);

    expect(navigation.followOutput).toBe(false);
  });
});

describe("waitForUserMessageElement", () => {
  it("stops waiting when the navigation is cancelled", async () => {
    vi.useFakeTimers();
    const scrollOwner = document.createElement("div");
    const shouldContinue = vi.fn().mockReturnValueOnce(true).mockReturnValue(false);

    const pending = waitForUserMessageElement(scrollOwner, USER_ONE_ID, shouldContinue);
    await vi.runAllTimersAsync();

    await expect(pending).resolves.toBeNull();
    expect(shouldContinue).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });
});

describe("VirtuosoMessageList destination navigation", () => {
  it("centers the current item index and highlights only its mounted prompt", async () => {
    const { container } = render(<VirtuosoMessageList {...props()} />);
    const destination = container.querySelector(USER_ONE_SELECTOR) as HTMLElement;

    await act(async () => navigation.options?.navigateTo(USER_ONE_ID));

    expect(navigation.scrollToIndex).toHaveBeenCalledWith({
      index: 0,
      align: "center",
      behavior: "smooth",
    });
    expect(destination.classList.contains(SEARCH_FLASH_CLASS)).toBe(true);
  });

  it("highlights the settled prompt when Virtuoso recycles an already mounted target", async () => {
    vi.useFakeTimers();
    const { container } = render(<VirtuosoMessageList {...props()} />);
    const original = container.querySelector(USER_ONE_SELECTOR) as HTMLElement;
    navigation.scrollToIndex.mockImplementationOnce(() => {
      requestAnimationFrame(() => {
        const replacement = original.cloneNode(true) as HTMLElement;
        replacement.classList.remove(SEARCH_FLASH_CLASS);
        original.replaceWith(replacement);
      });
    });

    let didNavigate: boolean | undefined;
    await act(async () => {
      const pending = navigation.options?.navigateTo(USER_ONE_ID);
      await vi.advanceTimersByTimeAsync(300);
      didNavigate = await pending;
    });

    const settled = container.querySelector(USER_ONE_SELECTOR) as HTMLElement;
    expect(didNavigate).toBe(true);
    expect(original.classList.contains(SEARCH_FLASH_CLASS)).toBe(false);
    expect(settled).not.toBe(original);
    expect(settled.classList.contains(SEARCH_FLASH_CLASS)).toBe(true);
    vi.useRealTimers();
  });

  it("retries when a mounted prompt has not reached its navigation position", async () => {
    vi.useFakeTimers();
    const { container } = render(<VirtuosoMessageList {...props()} />);
    const destination = container.querySelector(USER_ONE_SELECTOR) as HTMLElement;
    const scrollOwner = screen.getByTestId(SCROLL_OWNER_TEST_ID);
    Object.defineProperties(scrollOwner, {
      clientHeight: { configurable: true, value: 400 },
      scrollHeight: { configurable: true, value: 1200 },
    });
    scrollOwner.getBoundingClientRect = () => ({ top: 0, bottom: 400, height: 400 }) as DOMRect;
    let targetTop = 600;
    destination.getBoundingClientRect = () =>
      ({ top: targetTop, bottom: targetTop + 40, height: 40 }) as DOMRect;
    navigation.scrollToIndex.mockImplementation(() => {
      if (navigation.scrollToIndex.mock.calls.length === 2) targetTop = 180;
    });

    let didNavigate: boolean | undefined;
    await act(async () => {
      const pending = navigation.options?.navigateTo(USER_ONE_ID);
      await vi.advanceTimersByTimeAsync(1000);
      didNavigate = await pending;
    });

    expect(didNavigate).toBe(true);
    expect(navigation.scrollToIndex).toHaveBeenCalledTimes(2);
    expect(destination.classList.contains(SEARCH_FLASH_CLASS)).toBe(true);
    vi.useRealTimers();
  });

  it("restores the viewport when a virtualized destination does not mount", async () => {
    vi.useFakeTimers();
    const first = { id: USER_ONE_ID, author_type: "user", type: "message" } as Message;
    const second = { id: USER_TWO_ID, author_type: "user", type: "message" } as Message;
    const { container } = render(
      <VirtuosoMessageList
        {...props()}
        items={[
          { type: "message", message: first },
          { type: "message", message: second },
        ]}
        messages={[first, second]}
      />,
    );
    const scrollOwner = screen.getByTestId(SCROLL_OWNER_TEST_ID);
    scrollOwner.scrollTop = 240;
    navigation.scrollToIndex.mockImplementation(() => {
      scrollOwner.scrollTop = 900;
    });

    let didNavigate: boolean | undefined;
    await act(async () => {
      const pending = navigation.options?.navigateTo(USER_TWO_ID);
      await vi.runAllTimersAsync();
      didNavigate = await pending;
    });

    expect(didNavigate).toBe(false);
    expect(scrollOwner.scrollTop).toBe(240);
    expect(container.querySelector(`[data-user-message-id="${USER_TWO_ID}"]`)).toBeNull();
    vi.useRealTimers();
  });
});
