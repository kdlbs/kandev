"use client";
import * as React from "react";
import { useTranslation } from "react-i18next";
import { firstAnimationListValue, parseCssTime } from "@kandev/ui/animation-utils";

type GridSpinnerProps = {
  className?: string;
};

const gridKeyframes: Keyframe[] = [
  { offset: 0, transform: "scale3d(0.5, 0.5, 1)" },
  { offset: 0.35, transform: "scale3d(0, 0, 1)" },
  { offset: 0.7, transform: "scale3d(0.5, 0.5, 1)" },
  { offset: 1, transform: "scale3d(0.5, 0.5, 1)" },
];

const useCompositorEffect = typeof window === "undefined" ? React.useEffect : React.useLayoutEffect;

export function GridSpinner({ className }: GridSpinnerProps) {
  const { t } = useTranslation();
  const gridRef = React.useRef<HTMLSpanElement>(null);

  useCompositorEffect(() => {
    const cubes = Array.from(
      gridRef.current?.querySelectorAll<HTMLElement>(".spinner-grid-cube") ?? [],
    );
    const animations = startGridAnimations(cubes);
    if (!animations) return;

    for (const cube of cubes) cube.style.animation = "none";

    return () => {
      for (const animation of animations) animation.cancel();
      for (const cube of cubes) cube.style.removeProperty("animation");
    };
  }, []);

  return (
    <span
      ref={gridRef}
      className={`spinner-grid ${className ?? ""}`}
      role="status"
      aria-label={t("common:loadingIndicatorLabel")}
    >
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
    </span>
  );
}

function startGridAnimations(cubes: HTMLElement[]): Animation[] | null {
  if (cubes.length !== 9 || cubes.some((cube) => typeof cube.animate !== "function")) return null;

  const timing = cubes.map(readAnimationTiming);
  const validTiming = timing.filter((value): value is KeyframeAnimationOptions => value !== null);
  if (validTiming.length !== cubes.length) return null;

  const animations: Animation[] = [];
  try {
    for (let index = 0; index < cubes.length; index += 1) {
      animations.push(cubes[index].animate(gridKeyframes, validTiming[index]));
    }
    return animations;
  } catch {
    for (const animation of animations) animation.cancel();
    return null;
  }
}

function readAnimationTiming(cube: HTMLElement): KeyframeAnimationOptions | null {
  const computedStyle = window.getComputedStyle(cube);
  if (firstAnimationListValue(computedStyle.animationName) === "none") return null;

  const duration = parseCssTime(computedStyle.animationDuration);
  const delay = parseCssTime(computedStyle.animationDelay);
  const easing = firstAnimationListValue(computedStyle.animationTimingFunction);
  if (duration === null || duration <= 0 || delay === null || !easing) return null;

  return { delay, duration, easing, iterations: Infinity };
}
