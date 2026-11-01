import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CompositorSpin } from "@kandev/ui/compositor-spin";
import { GridSpinner } from "@/components/grid-spinner";

const originalVisibility = Object.getOwnPropertyDescriptor(document, "visibilityState");

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  if (originalVisibility) Object.defineProperty(document, "visibilityState", originalVisibility);
  else Reflect.deleteProperty(document, "visibilityState");
});

function motionEnvironment(reduced: boolean) {
  vi.useFakeTimers();
  const query = new EventTarget() as MediaQueryList;
  Object.defineProperty(query, "matches", { configurable: true, value: reduced });
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => query),
  );
  vi.spyOn(window, "getComputedStyle").mockReturnValue({
    animationName: "spin",
    animationDuration: "1.3s",
    animationDelay: "0s",
    animationTimingFunction: "ease-in-out",
  } as CSSStyleDeclaration);
  const handles: Animation[] = [];
  const animate = vi.fn(() => {
    const handle = {
      effect: {},
      playState: "running",
      cancel: vi.fn(() => {
        handle.playState = "idle";
      }),
      pause: vi.fn(() => {
        handle.playState = "paused";
      }),
      play: vi.fn(() => {
        handle.playState = "running";
      }),
    };
    handles.push(handle as unknown as Animation);
    return handle as unknown as Animation;
  });
  // jsdom has no Web Animations implementation.
  const descriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
  Object.defineProperty(HTMLElement.prototype, "animate", { configurable: true, value: animate });
  return {
    handles,
    animate,
    setReduced(matches: boolean) {
      act(() => {
        Object.defineProperty(query, "matches", { configurable: true, value: matches });
        query.dispatchEvent(new Event("change"));
        vi.runAllTimers();
      });
    },
    restore() {
      if (descriptor) Object.defineProperty(HTMLElement.prototype, "animate", descriptor);
      else Reflect.deleteProperty(HTMLElement.prototype, "animate");
    },
  };
}

for (const [name, Component, effects] of [
  ["grid", GridSpinner, 9],
  ["spin", CompositorSpin, 1],
] as const) {
  describe(`${name} motion preference`, () => {
    it("starts static, resumes normal motion, cancels on reduce, and removes listeners", () => {
      const env = motionEnvironment(true);
      try {
        const { unmount } = render(<Component />);
        act(() => vi.runAllTimers());
        expect(env.animate).not.toHaveBeenCalled();
        env.setReduced(false);
        expect(env.animate).toHaveBeenCalledTimes(effects);
        env.setReduced(true);
        for (const handle of env.handles) {
          expect(handle.cancel).toHaveBeenCalledOnce();
          expect(handle.effect).toBeNull();
        }
        env.setReduced(false);
        expect(env.animate).toHaveBeenCalledTimes(effects * 2);
        unmount();
        env.setReduced(true);
        env.setReduced(false);
        expect(env.animate).toHaveBeenCalledTimes(effects * 2);
      } finally {
        env.restore();
      }
    });

    it("keeps reduced motion static across hidden-to-visible transitions", () => {
      const env = motionEnvironment(false);
      try {
        render(<Component />);
        act(() => vi.runAllTimers());
        Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
        document.dispatchEvent(new Event("visibilitychange"));
        env.setReduced(true);
        Object.defineProperty(document, "visibilityState", {
          configurable: true,
          value: "visible",
        });
        document.dispatchEvent(new Event("visibilitychange"));
        for (const handle of env.handles) expect(handle.playState).toBe("idle");
        env.setReduced(false);
        expect(env.animate).toHaveBeenCalledTimes(effects * 2);
        for (const handle of env.handles.slice(effects)) expect(handle.playState).toBe("running");
      } finally {
        env.restore();
      }
    });

    it("cancels existing motion when the preference changes", () => {
      const env = motionEnvironment(false);
      try {
        render(<Component />);
        act(() => vi.runAllTimers());
        expect(env.animate).toHaveBeenCalledTimes(effects);
        env.setReduced(true);
        for (const handle of env.handles) expect(handle.playState).toBe("idle");
      } finally {
        env.restore();
      }
    });
  });
}

it("cancels grid promotion if reduced motion changes before the deferred task", () => {
  const env = motionEnvironment(false);
  try {
    render(<GridSpinner />);
    act(() => vi.advanceTimersToNextFrame());
    env.setReduced(true);
    expect(env.animate).not.toHaveBeenCalled();
    env.setReduced(false);
    expect(env.animate).toHaveBeenCalledTimes(9);
  } finally {
    env.restore();
  }
});

it("keeps hidden grid CSS fallback paused across preference changes", () => {
  const env = motionEnvironment(false);
  Object.defineProperty(HTMLElement.prototype, "animate", { configurable: true, value: undefined });
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
  try {
    const { container } = render(<GridSpinner />);
    act(() => vi.runAllTimers());
    const cubes = Array.from(container.querySelectorAll<HTMLElement>(".spinner-grid-cube"));
    expect(cubes).toHaveLength(9);
    expect(cubes.every((cube) => cube.style.animationPlayState === "paused")).toBe(true);
    env.setReduced(true);
    env.setReduced(false);
    expect(cubes.every((cube) => cube.style.animationPlayState === "paused")).toBe(true);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(cubes.every((cube) => cube.style.animationPlayState === "")).toBe(true);
  } finally {
    env.restore();
  }
});

it("reads spin timing from CSS when a presentation class changes", () => {
  const env = motionEnvironment(false);
  try {
    const view = render(<CompositorSpin className="opacity-50" />);
    vi.spyOn(window, "getComputedStyle").mockImplementation((element) => {
      const inline = (element as HTMLElement).style.animation;
      return {
        animationDuration: inline === "none" ? "0s" : "1.3s",
      } as CSSStyleDeclaration;
    });
    view.rerender(<CompositorSpin className="opacity-75" />);
    expect(env.animate).toHaveBeenLastCalledWith(
      expect.anything(),
      expect.objectContaining({ duration: 1300 }),
    );
  } finally {
    env.restore();
  }
});
