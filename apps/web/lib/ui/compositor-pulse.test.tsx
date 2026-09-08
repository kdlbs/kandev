import { cleanup, render } from "@testing-library/react";
import { CompositorPulse } from "@kandev/ui/compositor-pulse";
import { afterEach, describe, expect, it, vi } from "vitest";

const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, "matchMedia");

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalAnimate) {
    Object.defineProperty(HTMLElement.prototype, "animate", originalAnimate);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "animate");
  }
  if (originalMatchMedia) {
    Object.defineProperty(window, "matchMedia", originalMatchMedia);
  } else {
    Reflect.deleteProperty(window, "matchMedia");
  }
});

function installComputedAnimationStyles(overrides: Partial<CSSStyleDeclaration> = {}) {
  vi.spyOn(window, "getComputedStyle").mockReturnValue({
    animationDelay: "0s",
    animationDuration: "2s",
    animationName: "pulse",
    animationTimingFunction: "cubic-bezier(0.4, 0, 0.6, 1)",
    ...overrides,
  } as CSSStyleDeclaration);
}

function installAnimate() {
  const animation = { cancel: vi.fn() } as unknown as Animation;
  const animate = vi.fn(() => animation);
  Object.defineProperty(HTMLElement.prototype, "animate", {
    configurable: true,
    value: animate,
  });
  return { animate, animation };
}

function installReducedMotionMediaQuery(initialMatches = false) {
  let listener: ((event: MediaQueryListEvent) => void) | undefined;
  const mediaQuery = {
    matches: initialMatches,
    addEventListener: vi.fn((_type: string, nextListener: (event: MediaQueryListEvent) => void) => {
      listener = nextListener;
    }),
    removeEventListener: vi.fn(),
  } as unknown as MediaQueryList;
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn(() => mediaQuery),
  });

  return {
    mediaQuery,
    setMatches(matches: boolean) {
      Object.defineProperty(mediaQuery, "matches", { configurable: true, value: matches });
      listener?.({ matches } as MediaQueryListEvent);
    },
  };
}

describe("CompositorPulse", () => {
  it("replaces a CSS pulse with an infinite compositor opacity effect", () => {
    installComputedAnimationStyles();
    const { animate } = installAnimate();

    const { getByTestId } = render(
      <CompositorPulse data-testid="pulse" className="animate-pulse bg-primary" />,
    );

    const pulse = getByTestId("pulse");
    expect(pulse.className).toContain("animate-pulse");
    expect(animate).toHaveBeenCalledWith([{ opacity: 1 }, { opacity: 0.5 }, { opacity: 1 }], {
      delay: 0,
      duration: 2_000,
      easing: "cubic-bezier(0.4, 0, 0.6, 1)",
      iterations: Infinity,
    });
    expect(pulse.style.animation).toBe("none");
  });

  it("preserves a glow whose minimum opacity is at both endpoints", () => {
    installComputedAnimationStyles({ animationDuration: "3s" });
    const { animate } = installAnimate();

    render(
      <CompositorPulse
        className="chat-input-glow-running"
        minimumOpacity={0.4}
        minimumAtEndpoints
      />,
    );

    expect(animate).toHaveBeenCalledWith(
      [{ opacity: 0.4 }, { opacity: 1 }, { opacity: 0.4 }],
      expect.objectContaining({ duration: 3_000 }),
    );
  });

  it("keeps the animated CSS fallback when Web Animations is unavailable", () => {
    Reflect.deleteProperty(HTMLElement.prototype, "animate");

    const { getByTestId } = render(
      <CompositorPulse data-testid="pulse" className="animate-pulse" />,
    );

    expect(getByTestId("pulse").style.animation).toBe("");
  });

  it("does not override CSS reduced-motion suppression", () => {
    installComputedAnimationStyles({ animationName: "none" });
    const { animate } = installAnimate();

    const { getByTestId } = render(
      <CompositorPulse data-testid="pulse" className="motion-reduce:animate-none" />,
    );

    expect(animate).not.toHaveBeenCalled();
    expect(getByTestId("pulse").style.animation).toBe("");
  });

  it("cancels and recreates its effect when reduced motion changes", () => {
    installComputedAnimationStyles();
    const { animate, animation } = installAnimate();
    const { mediaQuery, setMatches } = installReducedMotionMediaQuery();

    const { getByTestId, unmount } = render(
      <CompositorPulse data-testid="pulse" className="animate-pulse" />,
    );
    const pulse = getByTestId("pulse");

    expect(mediaQuery.addEventListener).toHaveBeenCalledWith("change", expect.any(Function));
    setMatches(true);
    expect(animation.cancel).toHaveBeenCalledOnce();
    expect(pulse.style.animation).toBe("");

    setMatches(false);
    expect(animate).toHaveBeenCalledTimes(2);
    expect(pulse.style.animation).toBe("none");

    unmount();
    expect(mediaQuery.removeEventListener).toHaveBeenCalledWith("change", expect.any(Function));
  });

  it("cancels its effect and restores CSS on cleanup", () => {
    installComputedAnimationStyles();
    const { animation } = installAnimate();

    const { getByTestId, unmount } = render(
      <CompositorPulse data-testid="pulse" className="animate-pulse" />,
    );
    const pulse = getByTestId("pulse");
    unmount();

    expect(animation.cancel).toHaveBeenCalledOnce();
    expect(pulse.style.animation).toBe("");
  });
});
