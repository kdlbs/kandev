import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GridSpinner } from "./grid-spinner";

const delays = ["0.2s", "0.3s", "0.4s", "0.1s", "0.2s", "0.3s", "0s", "0.1s", "0.2s"];
const CUBE_SELECTOR = ".spinner-grid-cube";
const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
const originalVisibilityState = Object.getOwnPropertyDescriptor(document, "visibilityState");

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  if (originalAnimate) {
    Object.defineProperty(HTMLElement.prototype, "animate", originalAnimate);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "animate");
  }
  if (originalVisibilityState) {
    Object.defineProperty(document, "visibilityState", originalVisibilityState);
  } else {
    Reflect.deleteProperty(document, "visibilityState");
  }
});

function flushAnimationSetup() {
  act(() => vi.runAllTimers());
}

function installComputedAnimationStyles(easing = "ease-in-out") {
  vi.spyOn(window, "getComputedStyle").mockImplementation((element) => {
    const cubeIndex = Array.from(element.parentElement?.children ?? []).indexOf(element);
    return {
      animationDelay: delays[cubeIndex] ?? "0s",
      animationDuration: "1.3s",
      animationName: "spinner-grid",
      animationTimingFunction: easing,
    } as CSSStyleDeclaration;
  });
}

function installAnimate(implementation?: (...args: unknown[]) => Animation) {
  const animate = vi.fn(implementation ?? (() => makeAnimation()));
  Object.defineProperty(HTMLElement.prototype, "animate", {
    configurable: true,
    value: animate,
  });
  return animate;
}

function makeAnimation() {
  let playState: AnimationPlayState = "running";
  return {
    get playState() {
      return playState;
    },
    cancel: vi.fn(() => {
      playState = "idle";
    }),
    pause: vi.fn(() => {
      playState = "paused";
    }),
    play: vi.fn(() => {
      playState = "running";
    }),
  } as unknown as Animation;
}

describe("GridSpinner deferred setup", () => {
  it("keeps CSS motion until after the first frame without synchronous style reads", () => {
    installComputedAnimationStyles();
    const animate = installAnimate();
    const { container } = render(<GridSpinner />);

    expect(window.getComputedStyle).not.toHaveBeenCalled();
    expect(animate).not.toHaveBeenCalled();
    expect(container.querySelector<HTMLElement>(CUBE_SELECTOR)?.style.animation).toBe("");

    flushAnimationSetup();
    expect(animate).toHaveBeenCalledTimes(9);
  });

  it("cancels deferred setup if the spinner disappears before the first frame", () => {
    installComputedAnimationStyles();
    const animate = installAnimate();
    const { unmount } = render(<GridSpinner />);
    unmount();
    flushAnimationSetup();

    expect(window.getComputedStyle).not.toHaveBeenCalled();
    expect(animate).not.toHaveBeenCalled();
  });

  it("cancels promotion when unmounted between the frame and its deferred task", () => {
    installComputedAnimationStyles();
    const animate = installAnimate();
    const view = render(<GridSpinner />);
    act(() => vi.advanceTimersToNextFrame());
    expect(animate).not.toHaveBeenCalled();
    view.unmount();
    flushAnimationSetup();
    expect(window.getComputedStyle).not.toHaveBeenCalled();
    expect(animate).not.toHaveBeenCalled();
  });

  it("keeps newly promoted effects paused if visibility changed before promotion", () => {
    installComputedAnimationStyles();
    const animations = Array.from({ length: 9 }, () => makeAnimation());
    let animationIndex = 0;
    installAnimate(() => animations[animationIndex++]);
    render(<GridSpinner />);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
    flushAnimationSetup();
    for (const animation of animations) expect(animation.playState).toBe("paused");
  });
});

describe("GridSpinner", () => {
  it("pauses and resumes all owned cube effects with document visibility", () => {
    installComputedAnimationStyles();
    const animations = Array.from({ length: 9 }, () => makeAnimation());
    let animationIndex = 0;
    installAnimate(() => animations[animationIndex++]);

    const { container } = render(<GridSpinner />);
    flushAnimationSetup();
    const cubes = Array.from(container.querySelectorAll<HTMLElement>(CUBE_SELECTOR));
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));

    for (const animation of animations) expect(animation.pause).toHaveBeenCalledOnce();
    expect(cubes.every((cube) => cube.style.animationPlayState === "paused")).toBe(true);

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));

    for (const animation of animations) expect(animation.play).toHaveBeenCalledOnce();
    expect(cubes.every((cube) => cube.style.animationPlayState === "")).toBe(true);
  });

  it("starts nine compositor transform effects with the CSS timing and stagger", () => {
    installComputedAnimationStyles();
    const animate = installAnimate();

    const { container } = render(<GridSpinner className="text-primary" />);
    flushAnimationSetup();

    const status = container.querySelector<HTMLElement>('[role="status"]');
    expect(status?.getAttribute("aria-label")).toBe("Loading");
    expect(status?.className).toContain("text-primary");
    const cubes = Array.from(container.querySelectorAll<HTMLElement>(CUBE_SELECTOR));
    expect(cubes).toHaveLength(9);
    expect(animate).toHaveBeenCalledTimes(9);
    expect(animate.mock.calls.map((call) => call[0])).toEqual(
      Array.from({ length: 9 }, () => [
        { offset: 0, transform: "scale3d(0.5, 0.5, 1)" },
        { offset: 0.35, transform: "scale3d(0, 0, 1)" },
        { offset: 0.7, transform: "scale3d(0.5, 0.5, 1)" },
        { offset: 1, transform: "scale3d(0.5, 0.5, 1)" },
      ]),
    );
    expect(animate.mock.calls.map((call) => call[1])).toEqual(
      [200, 300, 400, 100, 200, 300, 0, 100, 200].map((delay) => ({
        delay,
        duration: 1_300,
        easing: "ease-in-out",
        iterations: Infinity,
      })),
    );
    expect(cubes.every((cube) => cube.style.animation === "none")).toBe(true);
  });

  it("keeps the CSS animations when Web Animations is unavailable", () => {
    Reflect.deleteProperty(HTMLElement.prototype, "animate");

    const { container } = render(<GridSpinner />);
    flushAnimationSetup();

    const cubes = Array.from(container.querySelectorAll<HTMLElement>(CUBE_SELECTOR));
    expect(cubes).toHaveLength(9);
    expect(cubes.every((cube) => cube.style.animation === "")).toBe(true);

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(cubes.every((cube) => cube.style.animationPlayState === "paused")).toBe(true);

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(cubes.every((cube) => cube.style.animationPlayState === "")).toBe(true);
  });
});

describe("GridSpinner lifecycle", () => {
  it("preserves commas inside CSS easing functions", () => {
    installComputedAnimationStyles("cubic-bezier(0.4, 0, 0.6, 1)");
    const animate = installAnimate();

    render(<GridSpinner />);
    flushAnimationSetup();

    expect(animate.mock.calls[0]?.[1]).toEqual({
      delay: 200,
      duration: 1_300,
      easing: "cubic-bezier(0.4, 0, 0.6, 1)",
      iterations: Infinity,
    });
  });

  it("keeps the animation phase when its presentation class changes", () => {
    installComputedAnimationStyles();
    const animations = Array.from({ length: 9 }, () => makeAnimation());
    let animationIndex = 0;
    const animate = installAnimate(() => animations[animationIndex++]);

    const { rerender } = render(<GridSpinner className="opacity-50" />);
    flushAnimationSetup();
    rerender(<GridSpinner className="opacity-75" />);

    expect(animate).toHaveBeenCalledTimes(9);
    for (const animation of animations) expect(animation.cancel).not.toHaveBeenCalled();
  });

  it("restores one consistent CSS fallback when setup fails partway", () => {
    installComputedAnimationStyles();
    const firstAnimation = makeAnimation();
    const animate = installAnimate(() => {
      if (animate.mock.calls.length === 2) throw new Error("animation setup failed");
      return firstAnimation;
    });

    const { container } = render(<GridSpinner />);
    flushAnimationSetup();

    const cubes = Array.from(container.querySelectorAll<HTMLElement>(CUBE_SELECTOR));
    expect(animate).toHaveBeenCalledTimes(2);
    expect(firstAnimation.cancel).toHaveBeenCalledOnce();
    expect(cubes.every((cube) => cube.style.animation === "")).toBe(true);

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(cubes.every((cube) => cube.style.animationPlayState === "paused")).toBe(true);

    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(cubes.every((cube) => cube.style.animationPlayState === "")).toBe(true);
  });

  it("cancels every compositor effect when it unmounts", () => {
    installComputedAnimationStyles();
    const animations = Array.from({ length: 9 }, () => makeAnimation());
    let animationIndex = 0;
    installAnimate(() => animations[animationIndex++]);

    const { unmount } = render(<GridSpinner />);
    flushAnimationSetup();
    unmount();

    for (const animation of animations) {
      expect(animation.cancel).toHaveBeenCalledOnce();
    }
  });
});
