import * as React from "react";
import { firstAnimationListValue, parseCssTime } from "./animation-utils";

type CompositorPulseProps = React.ComponentProps<"span"> & {
  minimumOpacity?: number;
  minimumAtEndpoints?: boolean;
};

const useCompositorEffect = typeof window === "undefined" ? React.useEffect : React.useLayoutEffect;

function CompositorPulse({
  minimumOpacity = 0.5,
  minimumAtEndpoints = false,
  ...props
}: CompositorPulseProps) {
  const elementRef = React.useRef<HTMLSpanElement>(null);

  useCompositorEffect(() => {
    const element = elementRef.current;
    if (!element || typeof element.animate !== "function") return;

    const inlineAnimation = element.style.animation;
    let animation: Animation | null = null;
    let reducedMotionQuery: MediaQueryList | null = null;
    try {
      reducedMotionQuery = window.matchMedia?.("(prefers-reduced-motion: reduce)") ?? null;
    } catch {
      // An unavailable media-query implementation does not block the CSS or
      // Web Animations paths.
    }

    const restoreAnimation = () => {
      animation?.cancel();
      animation = null;
      element.style.animation = inlineAnimation;
    };

    const startAnimation = () => {
      if (animation || reducedMotionQuery?.matches) return;

      const computedStyle = window.getComputedStyle(element);
      if (firstAnimationListValue(computedStyle.animationName) === "none") return;

      const duration = parseCssTime(computedStyle.animationDuration);
      const delay = parseCssTime(computedStyle.animationDelay);
      const easing = firstAnimationListValue(computedStyle.animationTimingFunction);
      if (duration === null || duration <= 0 || delay === null || !easing) return;

      const boundedMinimum = Math.min(1, Math.max(0, minimumOpacity));
      const opacityKeyframes = minimumAtEndpoints
        ? [{ opacity: boundedMinimum }, { opacity: 1 }, { opacity: boundedMinimum }]
        : [{ opacity: 1 }, { opacity: boundedMinimum }, { opacity: 1 }];
      try {
        animation = element.animate(opacityKeyframes, {
          delay,
          duration,
          easing,
          iterations: Infinity,
        });
      } catch {
        return;
      }

      element.style.animation = "none";
    };

    const handleMotionPreferenceChange = () => {
      restoreAnimation();
      startAnimation();
    };

    startAnimation();
    reducedMotionQuery?.addEventListener("change", handleMotionPreferenceChange);

    return () => {
      reducedMotionQuery?.removeEventListener("change", handleMotionPreferenceChange);
      restoreAnimation();
    };
  }, [minimumAtEndpoints, minimumOpacity, props.className]);

  return <span {...props} ref={elementRef} data-compositor-pulse="" />;
}

export { CompositorPulse };
export type { CompositorPulseProps };
