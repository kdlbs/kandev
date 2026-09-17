import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createChatScrollMotion } from "./chat-scroll-motion";
let frames: Map<number, FrameRequestCallback>;
let now: number;
function advance(ms = 16) {
  now += ms;
  const pending = [...frames.values()];
  frames.clear();
  pending.forEach((cb) => cb(now));
}
function fixture() {
  const el = document.createElement("div");
  let height = 1000;
  Object.defineProperties(el, {
    scrollHeight: { configurable: true, get: () => height },
    clientHeight: { value: 200 },
  });
  return {
    el,
    grow: (next: number) => {
      height = next;
    },
  };
}
beforeEach(() => {
  frames = new Map();
  now = 0;
  let id = 0;
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    frames.set(++id, cb);
    return id;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
});
afterEach(() => vi.unstubAllGlobals());
// @covers AC-UI-CHAT-MOTION-003.1, AC-UI-CHAT-MOTION-003.2, AC-UI-CHAT-MOTION-003.4
describe("interruptible chat following", () => {
  it("defers geometry reads and coalesces requests into a single moving target", () => {
    const { el, grow } = fixture();
    const read = vi.spyOn(el, "scrollHeight", "get");
    const motion = createChatScrollMotion(el, () => true, vi.fn());
    motion.request();
    motion.request();
    expect(read).not.toHaveBeenCalled();
    expect(frames.size).toBe(1);
    advance();
    advance();
    expect(el.scrollTop).toBeGreaterThan(0);
    expect(el.scrollTop).toBeLessThan(800);
    grow(1400);
    motion.request();
    expect(frames.size).toBe(1);
    for (let i = 0; i < 20; i++) advance();
    expect(el.scrollTop).toBe(1200);
    expect(frames.size).toBe(0);
    expect(motion.isRunning()).toBe(false);
    motion.dispose();
  });
  it.each(["wheel", "touchstart", "pointerdown", "keydown"])(
    "yields to %s and removes listeners on disposal",
    (type) => {
      const { el } = fixture();
      const interrupt = vi.fn();
      const motion = createChatScrollMotion(el, () => true, interrupt);
      motion.request();
      advance();
      advance();
      const top = el.scrollTop;
      el.dispatchEvent(
        type === "keydown" ? new KeyboardEvent(type, { key: "PageUp" }) : new Event(type),
      );
      advance();
      expect(el.scrollTop).toBe(top);
      expect(frames.size).toBe(0);
      expect(interrupt).toHaveBeenCalledTimes(1);
      motion.dispose();
      el.dispatchEvent(new Event("wheel"));
      expect(interrupt).toHaveBeenCalledTimes(1);
    },
  );
  it("does not treat typing as a scroll gesture", () => {
    const { el } = fixture();
    const interrupt = vi.fn();
    const motion = createChatScrollMotion(el, () => true, interrupt);
    motion.request();
    el.dispatchEvent(new KeyboardEvent("keydown", { key: "a" }));
    expect(interrupt).not.toHaveBeenCalled();
    expect(frames.size).toBe(1);
    motion.dispose();
  });
  it("cancels before writing when ownership changes", () => {
    const { el } = fixture();
    let allowed = true;
    const motion = createChatScrollMotion(el, () => allowed, vi.fn());
    motion.request();
    advance();
    allowed = false;
    const top = el.scrollTop;
    advance();
    expect(el.scrollTop).toBe(top);
    expect(frames.size).toBe(0);
    motion.dispose();
  });
});
