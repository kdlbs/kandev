import * as React from "react";
import { releaseAnimation } from "./animation-utils";
import { cn } from "./lib/utils";
import { createPersistentMotionVisibility } from "./persistent-motion-visibility";

const DEFAULT_DURATION_MS = 1_000;
const useCompositorEffect = typeof window === "undefined" ? React.useEffect : React.useLayoutEffect;

/**
 * Keeps a persistent rotation on an HTML transform target so its SVG child can
 * remain static and be promoted without per-frame SVG style work.
 */
function CompositorSpin({ className, style, ...props }: React.ComponentProps<"span">) {
  const elementRef = React.useRef<HTMLSpanElement>(null);
  const [compositorReady, setCompositorReady] = React.useState(false);

  useCompositorEffect(() => {
    const element = elementRef.current;
    if (!element) return;

    const visibility = createPersistentMotionVisibility(element);
    const registration = visibility.register(element);
    if (typeof element.animate !== "function") {
      return () => {
        registration.unregister();
        visibility.dispose();
      };
    }

    const inlineAnimation = element.style.animation;
    const inlineTransform = element.style.transform;
    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)");
    let animation: Animation | null = null;
    const restore = () => {
      if (animation) releaseAnimation(animation);
      animation = null;
      registration.setAnimation(null);
      element.style.animation = inlineAnimation;
      element.style.transform = inlineTransform;
    };
    const updateMotion = () => {
      restore();
      setCompositorReady(false);
      if (reducedMotion?.matches) return;
      element.style.removeProperty("animation");
      const duration = parseAnimationDuration(window.getComputedStyle(element).animationDuration);
      element.style.animation = "none";
      void window.getComputedStyle(element).animationName;
      element.style.transform = "translateZ(0)";
      try {
        animation = element.animate(
          [{ transform: "rotate(0deg)" }, { transform: "rotate(360deg)" }],
          { duration, easing: "linear", iterations: Infinity },
        );
        registration.setAnimation(animation);
        setCompositorReady(true);
      } catch {
        restore();
      }
    };
    updateMotion();
    reducedMotion?.addEventListener("change", updateMotion);

    return () => {
      reducedMotion?.removeEventListener("change", updateMotion);
      restore();
      registration.unregister();
      visibility.dispose();
    };
  }, [className]);

  return (
    <span
      {...props}
      ref={elementRef}
      className={cn("inline-flex animate-spin will-change-transform", className)}
      data-compositor-spin=""
      style={compositorReady ? { ...style, animation: "none", transform: "translateZ(0)" } : style}
    />
  );
}

function parseAnimationDuration(value: string): number {
  const duration = value.split(",")[0]?.trim();
  if (!duration) return DEFAULT_DURATION_MS;

  const numericValue = Number.parseFloat(duration);
  if (!Number.isFinite(numericValue)) return DEFAULT_DURATION_MS;
  return duration.endsWith("ms") ? numericValue : numericValue * 1_000;
}

export { CompositorSpin };
