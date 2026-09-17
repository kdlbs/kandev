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
});
describe("continuous growth and content interactions", () => {
  it("keeps moving when content grows on every frame", () => {
    const { el, grow } = fixture();
    const motion = createChatScrollMotion(el, () => true, vi.fn());
    motion.request();
    advance();
    for (let i = 1; i <= 30; i++) {
      const previous = el.scrollTop;
      grow(1000 + i * 10);
      motion.request();
      advance();
      expect(el.scrollTop).toBeGreaterThan(previous);
      expect(frames.size).toBe(1);
    }
    advance(180);
    expect(el.scrollTop).toBe(1100);
    expect(frames.size).toBe(0);
    motion.dispose();
  });
  it("keeps following after ordinary content clicks", () => {
    const { el } = fixture();
    const button = document.createElement("button");
    el.append(button);
    const interrupt = vi.fn();
    const motion = createChatScrollMotion(el, () => true, interrupt);
    motion.request();
    button.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    el.dispatchEvent(new MouseEvent("pointerdown"));
    expect(interrupt).not.toHaveBeenCalled();
    advance();
    advance();
    expect(el.scrollTop).toBeGreaterThan(0);
    motion.dispose();
  });
  it("yields to a scrollbar press and removes its listener", () => {
    const { el } = fixture();
    Object.defineProperties(el, { clientWidth: { value: 200 }, offsetWidth: { value: 216 } });
    vi.spyOn(el, "getBoundingClientRect").mockReturnValue({ left: 10, right: 226 } as DOMRect);
    const interrupt = vi.fn();
    const motion = createChatScrollMotion(el, () => true, interrupt);
    motion.request();
    el.dispatchEvent(new MouseEvent("pointerdown", { clientX: 215 }));
    expect(interrupt).toHaveBeenCalledOnce();
    expect(frames.size).toBe(0);
    motion.dispose();
    el.dispatchEvent(new MouseEvent("pointerdown", { clientX: 215 }));
    expect(interrupt).toHaveBeenCalledOnce();
  });
});
describe("scroll ownership", () => {
  it.each(["wheel", "keydown"])("yields to %s and removes listeners on disposal", (type) => {
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
  });
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

describe("touch scroll intent", () => {
  it("ignores taps and small finger jitter but yields to a drag", () => {
    const { el } = fixture();
    const interrupt = vi.fn();
    const motion = createChatScrollMotion(el, () => true, interrupt);
    const touch = (type: string, y: number) =>
      el.dispatchEvent(new TouchEvent(type, { touches: [{ clientY: y } as Touch] }));
    motion.request();
    touch("touchstart", 100);
    touch("touchmove", 102);
    el.dispatchEvent(new TouchEvent("touchend"));
    expect(interrupt).not.toHaveBeenCalled();
    expect(frames.size).toBe(1);
    touch("touchstart", 100);
    touch("touchmove", 112);
    expect(interrupt).toHaveBeenCalledOnce();
    expect(frames.size).toBe(0);
    touch("touchmove", 130);
    expect(interrupt).toHaveBeenCalledOnce();
    motion.dispose();
    touch("touchstart", 100);
    touch("touchmove", 130);
    expect(interrupt).toHaveBeenCalledOnce();
  });
});

describe("keyboard control ownership", () => {
  it.each(["button", "input", "select", "textarea", "a", "editable", "role-button"])(
    "keeps following when a %s handles a scroll key",
    (kind) => {
      const { el } = fixture();
      const control = document.createElement(
        kind === "editable" || kind === "role-button" ? "div" : kind,
      );
      if (kind === "editable") control.setAttribute("contenteditable", "true");
      if (kind === "role-button") control.setAttribute("role", "button");
      if (kind === "a") control.setAttribute("href", "#target");
      const target = document.createElement("span");
      control.append(target);
      el.append(control);
      const interrupt = vi.fn();
      const motion = createChatScrollMotion(el, () => true, interrupt);
      motion.request();
      target.dispatchEvent(new KeyboardEvent("keydown", { key: " ", bubbles: true }));
      expect(interrupt).not.toHaveBeenCalled();
      expect(motion.isRunning()).toBe(true);
      el.dispatchEvent(new KeyboardEvent("keydown", { key: "PageUp" }));
      expect(interrupt).toHaveBeenCalledOnce();
      motion.dispose();
    },
  );
});

it("leaves a prevented scroll key with its existing handler", () => {
  const { el } = fixture();
  const interrupt = vi.fn();
  const motion = createChatScrollMotion(el, () => true, interrupt);
  motion.request();
  const event = new KeyboardEvent("keydown", { key: "PageUp", cancelable: true });
  event.preventDefault();
  el.dispatchEvent(event);
  expect(interrupt).not.toHaveBeenCalled();
  expect(motion.isRunning()).toBe(true);
  motion.dispose();
});
