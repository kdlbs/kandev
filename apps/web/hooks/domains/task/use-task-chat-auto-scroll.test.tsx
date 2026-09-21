import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, renderHook } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { useTaskChatAutoScroll } from "./use-task-chat-auto-scroll";

afterEach(() => cleanup());

function scrollContainer() {
  const container = document.createElement("section");
  Object.defineProperties(container, {
    scrollHeight: { value: 2400, configurable: true },
    clientHeight: { value: 600 },
  });
  return container;
}

describe("task chat scroll intent", () => {
  it("opens an existing conversation at the bottom and resets when switching tasks", () => {
    const scrollParent = scrollContainer();
    const view = renderHook((id) => useTaskChatAutoScroll(scrollParent, [], id, 1), {
      initialProps: "task-1",
      wrapper: StateProvider,
    });
    expect(scrollParent.scrollTop).toBe(2400);
    scrollParent.scrollTop = 100;
    fireEvent.scroll(scrollParent);
    view.rerender("task-2");
    expect(scrollParent.scrollTop).toBe(2400);
  });

  it("follows comments arriving after mount until the user scrolls up", () => {
    const scrollParent = scrollContainer();
    scrollParent.scrollTop = 1800;
    const view = renderHook((count) => useTaskChatAutoScroll(scrollParent, [], "task-1", count), {
      initialProps: 0,
      wrapper: StateProvider,
    });
    Object.defineProperty(scrollParent, "scrollHeight", { value: 3000 });
    view.rerender(1);
    expect(scrollParent.scrollTop).toBe(3000);
    scrollParent.scrollTop = 100;
    fireEvent.scroll(scrollParent);
    Object.defineProperty(scrollParent, "scrollHeight", { value: 3600 });
    view.rerender(2);
    expect(scrollParent.scrollTop).toBe(100);
  });

  it("does not override a link to an older comment", () => {
    const scrollParent = scrollContainer();
    window.history.replaceState(null, "", "#comment-earlier");
    try {
      renderHook(() => useTaskChatAutoScroll(scrollParent, [], "task-1", 0), {
        wrapper: StateProvider,
      });
      expect(scrollParent.scrollTop).toBe(0);
    } finally {
      window.history.replaceState(null, "", window.location.pathname);
    }
  });
});
