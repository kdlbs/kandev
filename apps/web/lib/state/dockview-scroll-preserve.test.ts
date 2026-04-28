import { describe, it, expect, beforeEach, vi } from "vitest";

// Fake store API exposed by the mock below. Tests drive it directly to simulate
// the dockview store's setPendingChatScrollTop / subscribe / getState surface.
type Listener = (state: { isRestoringLayout: boolean }) => void;
const fakeStore = {
  isRestoringLayout: false,
  pendingChatScrollTop: null as number | null,
  setPendingChatScrollTop: vi.fn((v: number | null) => {
    fakeStore.pendingChatScrollTop = v;
  }),
  listeners: new Set<Listener>(),
  emit() {
    for (const l of fakeStore.listeners) l({ isRestoringLayout: fakeStore.isRestoringLayout });
  },
};

vi.mock("./dockview-store", () => ({
  useDockviewStore: {
    getState: () => fakeStore,
    subscribe: (listener: Listener) => {
      fakeStore.listeners.add(listener);
      return () => fakeStore.listeners.delete(listener);
    },
  },
}));

import { preserveChatScrollDuringLayout } from "./dockview-scroll-preserve";

function flushRaf(): Promise<void> {
  // happy-dom's requestAnimationFrame is asynchronous; flush via a microtask
  // followed by a 0ms timeout, which is enough for a single rAF callback.
  return new Promise((resolve) => setTimeout(resolve, 16));
}

function makeChatList(scrollTop: number): HTMLElement {
  const el = document.createElement("div");
  el.className = "chat-message-list";
  // happy-dom does not enforce overflow; assign scrollTop directly.
  Object.defineProperty(el, "scrollTop", {
    value: scrollTop,
    writable: true,
    configurable: true,
  });
  document.body.appendChild(el);
  return el;
}

describe("preserveChatScrollDuringLayout", () => {
  beforeEach(() => {
    document.body.innerHTML = "";
    fakeStore.isRestoringLayout = false;
    fakeStore.pendingChatScrollTop = null;
    fakeStore.listeners.clear();
    fakeStore.setPendingChatScrollTop.mockClear();
  });

  it("captures the current scrollTop and stores it as pending", () => {
    makeChatList(250);

    preserveChatScrollDuringLayout();

    expect(fakeStore.setPendingChatScrollTop).toHaveBeenCalledWith(250);
    expect(fakeStore.pendingChatScrollTop).toBe(250);
  });

  it("uses 0 when no chat list element is present", () => {
    preserveChatScrollDuringLayout();
    expect(fakeStore.setPendingChatScrollTop).toHaveBeenCalledWith(0);
  });

  it("restores scrollTop and clears pending after isRestoringLayout flips to false", async () => {
    const el = makeChatList(250);
    fakeStore.isRestoringLayout = true;

    preserveChatScrollDuringLayout();

    // While restoring, the listener should not act.
    fakeStore.emit();
    expect(el.scrollTop).toBe(250);
    expect(fakeStore.pendingChatScrollTop).toBe(250);

    // Layout restore completes — listener fires and schedules the rAF restore.
    el.scrollTop = 0; // simulate dockview wiping the position during rebuild
    fakeStore.isRestoringLayout = false;
    fakeStore.emit();

    await flushRaf();

    expect(el.scrollTop).toBe(250);
    expect(fakeStore.setPendingChatScrollTop).toHaveBeenLastCalledWith(null);
    expect(fakeStore.listeners.size).toBe(0);
  });

  it("restores against the latest .chat-message-list element (handles re-mount)", async () => {
    const original = makeChatList(180);
    fakeStore.isRestoringLayout = true;

    preserveChatScrollDuringLayout();

    // Simulate the panel being torn down and a new one mounted before restore.
    original.remove();
    const replacement = makeChatList(0);

    fakeStore.isRestoringLayout = false;
    fakeStore.emit();

    await flushRaf();

    expect(replacement.scrollTop).toBe(180);
  });

  it("does not fire prematurely when set() runs before isRestoringLayout becomes true", async () => {
    // Mirrors the maximizeGroup() call sequence: caller invokes the helper while
    // isRestoringLayout is still false, then a non-layout set() (e.g.
    // captureLiveWidths → syncPinnedWidthsFromApi) emits to subscribers BEFORE
    // isRestoringLayout flips to true. The helper must wait for the real
    // false→true→false transition rather than firing on this premature emit.
    const el = makeChatList(250);
    preserveChatScrollDuringLayout();

    // Premature emit while isRestoringLayout is still false. Must not restore.
    fakeStore.emit();
    expect(fakeStore.listeners.size).toBe(1);
    expect(fakeStore.pendingChatScrollTop).toBe(250);

    // Real maximize sequence: flip to true, then back to false.
    fakeStore.isRestoringLayout = true;
    fakeStore.emit();
    el.scrollTop = 0; // simulate dockview wiping scroll during rebuild
    fakeStore.isRestoringLayout = false;
    fakeStore.emit();

    await flushRaf();

    expect(el.scrollTop).toBe(250);
    expect(fakeStore.setPendingChatScrollTop).toHaveBeenLastCalledWith(null);
    expect(fakeStore.listeners.size).toBe(0);
  });
});
