import { createElement, type ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  type UserMessageNavigationOptions,
  useUserMessageNavigation,
} from "./use-message-navigation";

const SESSION_ID = "sess-1";

function makeMessage(id: string, authorType: Message["author_type"]): Message {
  return {
    id,
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId("task-1"),
    author_type: authorType,
    content: "",
    type: "message",
    created_at: "2026-07-21T00:00:00Z",
  };
}

function messageItem(id: string, authorType: Message["author_type"]): RenderItem {
  return { type: "message", message: makeMessage(id, authorType) };
}

type TestNavigationOptions = UserMessageNavigationOptions & { oldestCursor: string | null };

function renderNavigation(items: RenderItem[], overrides: Partial<TestNavigationOptions> = {}) {
  function wrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, null, children);
  }
  const initialProps: TestNavigationOptions = {
    sessionId: SESSION_ID,
    items,
    hasOlder: false,
    oldestCursor: items[0]?.type === "message" ? items[0].message.id : null,
    loadOlder: vi.fn(async () => 0),
    navigateTo: vi.fn(async () => true),
    ...overrides,
  };
  return renderHook((props: TestNavigationOptions) => useUserMessageNavigation(props), {
    wrapper,
    initialProps,
  });
}

describe("useUserMessageNavigation", () => {
  it("orders every rendered user prompt and ignores non-user messages", () => {
    const { result } = renderNavigation([
      messageItem("u1", "user"),
      messageItem("a1", "agent"),
      {
        type: "turn_group",
        id: "activity-1",
        turnId: "turn-1",
        messages: [makeMessage("not-a-prompt", "user")],
      },
      messageItem("u2", "user"),
      messageItem("u3", "user"),
    ]);

    expect(result.current.userMessageIds).toEqual(["u1", "u2", "u3"]);
  });

  it("uses the newest rendered prompt until the viewport reports an origin", () => {
    const { result } = renderNavigation([messageItem("u1", "user"), messageItem("u2", "user")]);

    expect(result.current.originId).toBe("u2");
  });

  it("accepts viewport-origin updates", () => {
    const { result } = renderNavigation([messageItem("u1", "user")]);

    expect(result.current.setViewportOrigin).toEqual(expect.any(Function));
  });

  it("selects adjacent prompts from the reported viewport origin", () => {
    const { result } = renderNavigation([
      messageItem("u1", "user"),
      messageItem("u2", "user"),
      messageItem("u3", "user"),
    ]);

    act(() => result.current.setViewportOrigin("u2"));

    expect(result.current).toMatchObject({
      originId: "u2",
      hasPrevious: true,
      previousId: "u1",
      hasNext: true,
      nextId: "u3",
    });
  });

  it("exposes directional navigation actions", () => {
    const { result } = renderNavigation([messageItem("u1", "user")]);

    expect(result.current.goPrevious).toEqual(expect.any(Function));
    expect(result.current.goNext).toEqual(expect.any(Function));
  });

  it("navigates to the next loaded prompt without loading older history", async () => {
    const navigateTo = vi.fn(async () => true);
    const loadOlder = vi.fn(async () => 20);
    const { result } = renderNavigation([messageItem("u1", "user"), messageItem("u2", "user")], {
      navigateTo,
      loadOlder,
    });
    act(() => result.current.setViewportOrigin("u1"));

    await act(() => result.current.goNext());

    expect(navigateTo).toHaveBeenCalledWith("u2");
    expect(loadOlder).not.toHaveBeenCalled();
    expect(result.current.originId).toBe("u2");
  });
});

describe("useUserMessageNavigation pagination", () => {
  it("waits for prepended messages to commit after loadOlder resolves", async () => {
    vi.useFakeTimers();
    const navigateTo = vi.fn(async () => true);
    const loadOlder = vi.fn(async () => 20);
    const hook = renderNavigation([messageItem("u-current", "user")], {
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
      navigateTo,
    });

    let navigation!: Promise<void>;
    act(() => {
      navigation = hook.result.current.goPrevious();
    });
    await act(async () => Promise.resolve());
    expect(navigateTo).not.toHaveBeenCalled();

    hook.rerender({
      sessionId: SESSION_ID,
      items: [messageItem("u-old", "user"), messageItem("u-current", "user")],
      hasOlder: false,
      oldestCursor: "cursor-0",
      loadOlder,
      navigateTo,
    });
    await act(async () => vi.runAllTimersAsync());
    await act(() => navigation);

    expect(navigateTo).toHaveBeenCalledWith("u-old");
    vi.useRealTimers();
  });

  it("allows a virtualized page time to commit before deciding pagination made no progress", async () => {
    vi.useFakeTimers();
    const navigateTo = vi.fn(async () => true);
    const loadOlder = vi.fn(async () => 20);
    const hook = renderNavigation([messageItem("u-current", "user")], {
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
      navigateTo,
    });

    let navigation!: Promise<void>;
    act(() => {
      navigation = hook.result.current.goPrevious();
    });
    await act(async () => Promise.resolve());
    await act(async () => vi.advanceTimersByTimeAsync(250));

    hook.rerender({
      sessionId: SESSION_ID,
      items: [messageItem("u-old", "user"), messageItem("u-current", "user")],
      hasOlder: false,
      oldestCursor: "cursor-0",
      loadOlder,
      navigateTo,
    });
    await act(async () => vi.runAllTimersAsync());
    await act(() => navigation);

    expect(navigateTo).toHaveBeenCalledWith("u-old");
    vi.useRealTimers();
  });
});

describe("useUserMessageNavigation paginated actions", () => {
  it("loads successive older pages until it finds the previous user prompt", async () => {
    const navigateTo = vi.fn(async () => true);
    const pageResolvers: Array<(count: number) => void> = [];
    const loadOlder = vi.fn(() => new Promise<number>((resolve) => pageResolvers.push(resolve)));
    const hook = renderNavigation([messageItem("u-current", "user")], {
      hasOlder: true,
      oldestCursor: "cursor-2",
      loadOlder,
      navigateTo,
    });

    let navigation!: Promise<void>;
    act(() => {
      navigation = hook.result.current.goPrevious();
    });
    hook.rerender({
      sessionId: SESSION_ID,
      items: [messageItem("a-older", "agent"), messageItem("u-current", "user")],
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
      navigateTo,
    });
    await act(async () => pageResolvers[0](20));

    hook.rerender({
      sessionId: SESSION_ID,
      items: [
        messageItem("u-old", "user"),
        messageItem("a-older", "agent"),
        messageItem("u-current", "user"),
      ],
      hasOlder: false,
      oldestCursor: "cursor-0",
      loadOlder,
      navigateTo,
    });
    await act(async () => pageResolvers[1](20));
    await act(() => navigation);

    expect(loadOlder).toHaveBeenCalledTimes(2);
    expect(navigateTo).toHaveBeenCalledWith("u-old");
    expect(hook.result.current).toMatchObject({
      isBusy: false,
      originId: "u-old",
      hasPrevious: false,
    });
  });

  it("blocks duplicate actions while destination navigation is pending", async () => {
    let resolveNavigation!: (didNavigate: boolean) => void;
    const navigateTo = vi.fn(
      () => new Promise<boolean>((resolve) => (resolveNavigation = resolve)),
    );
    const { result } = renderNavigation([messageItem("u1", "user"), messageItem("u2", "user")], {
      navigateTo,
    });
    act(() => result.current.setViewportOrigin("u1"));

    let firstAction!: Promise<void>;
    act(() => {
      firstAction = result.current.goNext();
      void result.current.goNext();
    });

    expect(navigateTo).toHaveBeenCalledTimes(1);
    expect(result.current.isBusy).toBe(true);
    await act(async () => resolveNavigation(true));
    await act(() => firstAction);
    expect(result.current.isBusy).toBe(false);
  });
});

describe("useUserMessageNavigation boundaries and cancellation", () => {
  it("does not enable previous navigation without a rendered user prompt", () => {
    const { result } = renderNavigation([messageItem("a1", "agent")], {
      hasOlder: true,
      oldestCursor: "cursor-1",
    });

    expect(result.current.hasPrevious).toBe(false);
  });

  it("leaves previous enabled after a failed or no-progress page", async () => {
    const navigateTo = vi.fn(async () => true);
    const loadOlder = vi.fn(async () => 0);
    const { result } = renderNavigation([messageItem("u1", "user")], {
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
      navigateTo,
    });

    await act(() => result.current.goPrevious());

    expect(result.current).toMatchObject({
      originId: "u1",
      hasPrevious: true,
      isBusy: false,
    });
    expect(navigateTo).not.toHaveBeenCalled();
  });

  it("stops when a non-empty page leaves the cursor unchanged", async () => {
    let resolvePage!: (count: number) => void;
    const loadOlder = vi.fn(() => new Promise<number>((resolve) => (resolvePage = resolve)));
    const hook = renderNavigation([messageItem("u1", "user")], {
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
    });
    let navigation!: Promise<void>;
    act(() => {
      navigation = hook.result.current.goPrevious();
    });
    hook.rerender({
      sessionId: SESSION_ID,
      items: [messageItem("u1", "user")],
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
      navigateTo: vi.fn(async () => true),
    });

    await act(async () => resolvePage(20));
    await act(() => navigation);

    expect(loadOlder).toHaveBeenCalledTimes(1);
    expect(hook.result.current.hasPrevious).toBe(true);
  });

  it("discards a pending navigation when the active session changes", async () => {
    let resolvePage!: (count: number) => void;
    const navigateTo = vi.fn(async () => true);
    const loadOlder = vi.fn(() => new Promise<number>((resolve) => (resolvePage = resolve)));
    const hook = renderNavigation([messageItem("u1", "user")], {
      hasOlder: true,
      oldestCursor: "cursor-1",
      loadOlder,
      navigateTo,
    });
    let navigation!: Promise<void>;
    act(() => {
      navigation = hook.result.current.goPrevious();
    });
    hook.rerender({
      sessionId: "sess-2",
      items: [messageItem("u-new", "user")],
      hasOlder: false,
      oldestCursor: "u-new",
      loadOlder,
      navigateTo,
    });

    await act(async () => resolvePage(20));
    await act(() => navigation);

    expect(navigateTo).not.toHaveBeenCalled();
    expect(hook.result.current).toMatchObject({ originId: "u-new", isBusy: false });
  });
});
