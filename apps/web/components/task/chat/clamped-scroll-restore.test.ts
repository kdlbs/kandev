import { describe, expect, it, vi } from "vitest";
import { scheduleClampedScrollRestore } from "./clamped-scroll-restore";

function frameQueue() {
  const frames: FrameRequestCallback[] = [];
  return {
    requestFrame: (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    },
    runNext: () => frames.shift()?.(0),
    pending: () => frames.length,
  };
}

function clampedElement(maxScrollTop: () => number) {
  let scrollTop = 0;
  return {
    isConnected: true,
    get scrollTop() {
      return scrollTop;
    },
    set scrollTop(value: number) {
      scrollTop = Math.min(value, maxScrollTop());
    },
  };
}

describe("scheduleClampedScrollRestore", () => {
  it("re-applies the target after late layout growth makes it reachable", () => {
    const frames = frameQueue();
    let maxScrollTop = 100;
    const element = clampedElement(() => maxScrollTop);
    const onApply = vi.fn();
    const onComplete = vi.fn();

    scheduleClampedScrollRestore({
      element,
      targetScrollTop: 500,
      onApply,
      onComplete,
      requestFrame: frames.requestFrame,
    });
    frames.runNext();
    expect(element.scrollTop).toBe(100);

    maxScrollTop = 500;
    frames.runNext();

    expect(element.scrollTop).toBe(500);
    expect(onApply).toHaveBeenCalledTimes(2);
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(frames.pending()).toBe(0);
  });

  it("does not write after the target is detached", () => {
    const frames = frameQueue();
    const element = clampedElement(() => 0);
    element.isConnected = false;
    const onApply = vi.fn();
    const onComplete = vi.fn();

    scheduleClampedScrollRestore({
      element,
      targetScrollTop: 500,
      onApply,
      onComplete,
      requestFrame: frames.requestFrame,
    });
    frames.runNext();

    expect(onApply).not.toHaveBeenCalled();
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(frames.pending()).toBe(0);
  });

  it("stops after the configured frame budget when the target stays clamped", () => {
    const frames = frameQueue();
    const element = clampedElement(() => 100);
    const onApply = vi.fn();
    const onComplete = vi.fn();

    scheduleClampedScrollRestore({
      element,
      targetScrollTop: 500,
      onApply,
      onComplete,
      maxFrames: 3,
      requestFrame: frames.requestFrame,
    });
    while (frames.pending()) frames.runNext();

    expect(onApply).toHaveBeenCalledTimes(3);
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(element.scrollTop).toBe(100);
  });
});
