import * as React from "react";
import { cn } from "./lib/utils";

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
    if (!element || typeof element.animate !== "function") return;

    element.style.removeProperty("animation");
    const computedStyle = window.getComputedStyle(element);
    const duration = parseAnimationDuration(computedStyle.animationDuration);
    element.style.animation = "none";
    void window.getComputedStyle(element).animationName;
    element.style.transform = "translateZ(0)";
    const animation = element.animate(
      [{ transform: "rotate(0deg)" }, { transform: "rotate(360deg)" }],
      {
        duration,
        easing: "linear",
        iterations: Infinity,
      },
    );
    setCompositorReady(true);

    return () => animation.cancel();
  }, [className]);

  const compositorStyle = compositorReady
    ? { ...style, animation: "none", transform: "translateZ(0)" }
    : style;

  return (
    <span
      {...props}
      ref={elementRef}
      className={cn("inline-flex animate-spin will-change-transform", className)}
      style={compositorStyle}
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
