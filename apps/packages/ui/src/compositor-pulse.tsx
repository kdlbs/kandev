import * as React from "react";

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

    const computedStyle = window.getComputedStyle(element);
    if (computedStyle.animationName === "none") return;

    const duration = parseCssTime(computedStyle.animationDuration);
    const delay = parseCssTime(computedStyle.animationDelay);
    const easing = computedStyle.animationTimingFunction.trim();
    if (duration === null || duration <= 0 || delay === null || !easing) return;

    const boundedMinimum = Math.min(1, Math.max(0, minimumOpacity));
    const opacityKeyframes = minimumAtEndpoints
      ? [{ opacity: boundedMinimum }, { opacity: 1 }, { opacity: boundedMinimum }]
      : [{ opacity: 1 }, { opacity: boundedMinimum }, { opacity: 1 }];
    let animation: Animation;
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

    const inlineAnimation = element.style.animation;
    element.style.animation = "none";

    return () => {
      animation.cancel();
      element.style.animation = inlineAnimation;
    };
  }, [minimumAtEndpoints, minimumOpacity, props.className]);

  return <span {...props} ref={elementRef} data-compositor-pulse="" />;
}

function parseCssTime(value: string): number | null {
  const firstValue = value.split(",")[0]?.trim();
  if (!firstValue) return null;

  const numericValue = Number.parseFloat(firstValue);
  if (!Number.isFinite(numericValue)) return null;
  return firstValue.endsWith("ms") ? numericValue : numericValue * 1_000;
}

export { CompositorPulse };
export type { CompositorPulseProps };
