import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useChatScrollMotion } from "./use-chat-scroll-motion";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("animates allowed requests, settles on disable, and stops when hidden", () => {
  let id = 0;
  let time = 0;
  const frames = new Map<number, FrameRequestCallback>();
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    frames.set(++id, cb);
    return id;
  });
  vi.stubGlobal("cancelAnimationFrame", (key: number) => frames.delete(key));
  const el = document.createElement("div");
  Object.defineProperties(el, { scrollHeight: { value: 1000 }, clientHeight: { value: 200 } });
  const options = {
    scrollRef: { current: el },
    motionEnabled: true,
    enabled: true,
    isVisible: true,
    sessionId: "s",
    isNearBottomRef: { current: true },
    isBlocked: () => false,
    instant: (element: HTMLElement) => {
      element.scrollTop = 800;
    },
  };
  const hook = renderHook(useChatScrollMotion, { initialProps: options });
  act(() => hook.result.current.followBottom());
  const step = () =>
    act(() => {
      time += 16;
      const pending = [...frames.values()];
      frames.clear();
      pending.forEach((cb) => cb(time));
    });
  step();
  step();
  expect(el.scrollTop).toBeGreaterThan(0);
  expect(el.scrollTop).toBeLessThan(800);
  hook.rerender({ ...options, motionEnabled: false });
  expect(el.scrollTop).toBe(800);
  expect(frames.size).toBe(0);
  el.scrollTop = 0;
  hook.rerender(options);
  act(() => hook.result.current.followBottom());
  step();
  hook.rerender({ ...options, isVisible: false });
  step();
  expect(frames.size).toBe(0);
  hook.unmount();
});
