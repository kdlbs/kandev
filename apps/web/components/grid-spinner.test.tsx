import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GridSpinner } from "./grid-spinner";

const delays = ["0.2s", "0.3s", "0.4s", "0.1s", "0.2s", "0.3s", "0s", "0.1s", "0.2s"];
const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalAnimate) {
    Object.defineProperty(HTMLElement.prototype, "animate", originalAnimate);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "animate");
  }
});

function installComputedAnimationStyles() {
  vi.spyOn(window, "getComputedStyle").mockImplementation((element) => {
    const cubeIndex = Array.from(element.parentElement?.children ?? []).indexOf(element);
    return {
      animationDelay: delays[cubeIndex] ?? "0s",
      animationDuration: "1.3s",
      animationName: "spinner-grid",
      animationTimingFunction: "ease-in-out",
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
  return { cancel: vi.fn() } as unknown as Animation;
}

describe("GridSpinner", () => {
  it("starts nine compositor transform effects with the CSS timing and stagger", () => {
    installComputedAnimationStyles();
    const animate = installAnimate();

    const { container } = render(<GridSpinner className="text-primary" />);

    const status = container.querySelector<HTMLElement>('[role="status"]');
    expect(status?.getAttribute("aria-label")).toBe("Loading");
    expect(status?.className).toContain("text-primary");
    const cubes = Array.from(container.querySelectorAll<HTMLElement>(".spinner-grid-cube"));
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

    const cubes = Array.from(container.querySelectorAll<HTMLElement>(".spinner-grid-cube"));
    expect(cubes).toHaveLength(9);
    expect(cubes.every((cube) => cube.style.animation === "")).toBe(true);
  });

  it("restores one consistent CSS fallback when setup fails partway", () => {
    installComputedAnimationStyles();
    const firstAnimation = makeAnimation();
    const animate = installAnimate(() => {
      if (animate.mock.calls.length === 2) throw new Error("animation setup failed");
      return firstAnimation;
    });

    const { container } = render(<GridSpinner />);

    const cubes = Array.from(container.querySelectorAll<HTMLElement>(".spinner-grid-cube"));
    expect(animate).toHaveBeenCalledTimes(2);
    expect(firstAnimation.cancel).toHaveBeenCalledOnce();
    expect(cubes.every((cube) => cube.style.animation === "")).toBe(true);
  });

  it("cancels every compositor effect when it unmounts", () => {
    installComputedAnimationStyles();
    const animations = Array.from({ length: 9 }, () => makeAnimation());
    let animationIndex = 0;
    installAnimate(() => animations[animationIndex++]);

    const { unmount } = render(<GridSpinner />);
    unmount();

    for (const animation of animations) {
      expect(animation.cancel).toHaveBeenCalledOnce();
    }
  });
});
