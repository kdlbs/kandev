"use client";
import * as React from "react";
import { useTranslation } from "react-i18next";
import {
  firstAnimationListValue,
  parseCssTime,
  releaseAnimation,
} from "@kandev/ui/animation-utils";
import { createPersistentMotionVisibility } from "@kandev/ui/persistent-motion-visibility";

type GridSpinnerProps = {
  className?: string;
  ariaLabel?: string;
};

const gridKeyframes: Keyframe[] = [
  { offset: 0, transform: "scale3d(0.5, 0.5, 1)" },
  { offset: 0.35, transform: "scale3d(0, 0, 1)" },
  { offset: 0.7, transform: "scale3d(0.5, 0.5, 1)" },
  { offset: 1, transform: "scale3d(0.5, 0.5, 1)" },
];

const useCompositorEffect = typeof window === "undefined" ? React.useEffect : React.useLayoutEffect;

export function GridSpinner({ className, ariaLabel }: GridSpinnerProps) {
  const { t } = useTranslation();
  const gridRef = React.useRef<HTMLSpanElement>(null);

  useCompositorEffect(() => {
    const cubes = Array.from(
      gridRef.current?.querySelectorAll<HTMLElement>(".spinner-grid-cube") ?? [],
    );
    const grid = gridRef.current;
    if (!grid) return;
    const visibility = createPersistentMotionVisibility(grid);
    const registrations = cubes.map((cube) => visibility.register(cube));
    let animations: Animation[] | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let frame = 0;
    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)");
    const reset = () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
      for (const animation of animations ?? []) releaseAnimation(animation);
      animations = null;
      for (const registration of registrations) registration.setAnimation(null);
      for (const cube of cubes) cube.style.removeProperty("animation-name");
    };
    const updateMotion = () => {
      reset();
      if (reducedMotion?.matches) return;
      // CSS supplies the initial motion. Defer style reads past the first paint.
      frame = requestAnimationFrame(() => {
        timer = setTimeout(() => {
          animations = startGridAnimations(cubes);
          if (!animations) return;
          for (let index = 0; index < cubes.length; index += 1) {
            registrations[index]?.setAnimation(animations[index] ?? null);
            cubes[index].style.animationName = "none";
          }
        }, 0);
      });
    };
    updateMotion();
    reducedMotion?.addEventListener("change", updateMotion);

    return () => {
      reducedMotion?.removeEventListener("change", updateMotion);
      reset();
      for (const registration of registrations) registration.unregister();
      visibility.dispose();
      for (const cube of cubes) cube.style.removeProperty("animation-name");
    };
  }, []);

  return (
    <span
      ref={gridRef}
      className={`spinner-grid ${className ?? ""}`}
      role="status"
      aria-label={ariaLabel ?? t("common:loadingIndicatorLabel")}
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
    for (const animation of animations) releaseAnimation(animation);
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
