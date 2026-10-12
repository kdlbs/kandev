import { describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { getTaskStateIcon } from "./state-icons";

const SPIN_SELECTOR = ".animate-spin";

describe("CompositorSpin visibility", () => {
  it("pauses and resumes the compositor spinner with document visibility", () => {
    const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
    const originalVisibilityState = Object.getOwnPropertyDescriptor(document, "visibilityState");
    let playState: AnimationPlayState = "running";
    const animation = {
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
    Object.defineProperty(HTMLElement.prototype, "animate", {
      configurable: true,
      value: vi.fn(() => animation),
    });

    try {
      const { container } = render(
        <TooltipProvider>{getTaskStateIcon("IN_PROGRESS")}</TooltipProvider>,
      );
      const wrapper = container.querySelector(SPIN_SELECTOR) as HTMLElement;

      setDocumentVisibility("hidden");
      expect(animation.pause).toHaveBeenCalledOnce();
      expect(wrapper.style.animationPlayState).toBe("paused");

      setDocumentVisibility("visible");
      expect(animation.play).toHaveBeenCalledOnce();
      expect(wrapper.style.animationPlayState).toBe("");
    } finally {
      restoreProperty(HTMLElement.prototype, "animate", originalAnimate);
      restoreProperty(document, "visibilityState", originalVisibilityState);
    }
  });

  it("pauses the CSS fallback when Web Animations are unavailable", () => {
    const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
    Reflect.deleteProperty(HTMLElement.prototype, "animate");

    try {
      const { container } = render(
        <TooltipProvider>{getTaskStateIcon("IN_PROGRESS")}</TooltipProvider>,
      );
      const wrapper = container.querySelector(SPIN_SELECTOR) as HTMLElement;

      setDocumentVisibility("hidden");
      expect(wrapper.style.animationPlayState).toBe("paused");

      setDocumentVisibility("visible");
      expect(wrapper.style.animationPlayState).toBe("");
    } finally {
      restoreProperty(HTMLElement.prototype, "animate", originalAnimate);
    }
  });

  it("cleans up visibility control when compositor setup throws", () => {
    const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
    const animate = vi.fn(() => {
      throw new Error("animation setup failed");
    });
    Object.defineProperty(HTMLElement.prototype, "animate", {
      configurable: true,
      value: animate,
    });

    try {
      const { container } = render(
        <TooltipProvider>{getTaskStateIcon("IN_PROGRESS")}</TooltipProvider>,
      );
      const wrapper = container.querySelector(SPIN_SELECTOR) as HTMLElement;
      expect(animate).toHaveBeenCalledOnce();
      expect(wrapper.style.animation).toBe("");
    } finally {
      restoreProperty(HTMLElement.prototype, "animate", originalAnimate);
    }
  });
});

function setDocumentVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value: state,
  });
  document.dispatchEvent(new Event("visibilitychange"));
}

function restoreProperty(
  target: object,
  property: string,
  descriptor: PropertyDescriptor | undefined,
) {
  if (descriptor) Object.defineProperty(target, property, descriptor);
  else Reflect.deleteProperty(target, property);
}
