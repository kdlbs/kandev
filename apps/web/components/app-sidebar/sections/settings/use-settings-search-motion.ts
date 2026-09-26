"use client";

import { useEffect, useLayoutEffect, useRef } from "react";

const MOTION_SELECTOR = "[data-settings-search-motion-key]";
const MOTION_EASING = "cubic-bezier(0.2, 0, 0, 1)";
const ENTER_OPTIONS: KeyframeAnimationOptions = {
  duration: 140,
  easing: MOTION_EASING,
};
const MOVE_OPTIONS: KeyframeAnimationOptions = {
  duration: 160,
  easing: MOTION_EASING,
};

type MotionEntry = {
  key: string;
  node: HTMLElement;
  top: number;
};

export function useSettingsSearchMotion(layoutKey: string) {
  const containerRef = useRef<HTMLDivElement>(null);
  const previousPositionsRef = useRef(new Map<string, number>());
  const animationsRef = useRef(new Map<string, Animation>());

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const entries = collectMotionEntries(container);
    const nextPositions = new Map(entries.map((entry) => [entry.key, entry.top]));
    const animations = animationsRef.current;
    cancelRemovedAnimations(animations, new Set(nextPositions.keys()));

    if (prefersReducedMotion()) {
      cancelAnimations(animations);
      previousPositionsRef.current = nextPositions;
      return;
    }

    for (const entry of entries) {
      animations.get(entry.key)?.cancel();
      animations.delete(entry.key);
      const previousTop = previousPositionsRef.current.get(entry.key);
      const animation = animateEntry(entry, previousTop);
      if (!animation) continue;

      animations.set(entry.key, animation);
      animation.addEventListener(
        "finish",
        () => {
          if (animations.get(entry.key) === animation) animations.delete(entry.key);
        },
        { once: true },
      );
    }

    previousPositionsRef.current = nextPositions;
  }, [layoutKey]);

  useEffect(
    () => () => {
      cancelAnimations(animationsRef.current);
    },
    [],
  );

  return containerRef;
}

function collectMotionEntries(container: HTMLElement): MotionEntry[] {
  return [...container.querySelectorAll<HTMLElement>(MOTION_SELECTOR)].flatMap((node) => {
    const key = node.dataset.settingsSearchMotionKey;
    return key ? [{ key, node, top: node.getBoundingClientRect().top }] : [];
  });
}

function animateEntry(entry: MotionEntry, previousTop: number | undefined): Animation | null {
  if (typeof entry.node.animate !== "function") return null;
  if (previousTop === undefined) {
    return entry.node.animate(
      [
        { opacity: 0, transform: "translateY(2px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      ENTER_OPTIONS,
    );
  }

  const offset = previousTop - entry.top;
  if (Math.abs(offset) < 0.5) return null;
  return entry.node.animate(
    [{ transform: `translateY(${offset}px)` }, { transform: "translateY(0)" }],
    MOVE_OPTIONS,
  );
}

function cancelRemovedAnimations(animations: Map<string, Animation>, activeKeys: Set<string>) {
  for (const [key, animation] of animations) {
    if (activeKeys.has(key)) continue;
    animation.cancel();
    animations.delete(key);
  }
}

function cancelAnimations(animations: Map<string, Animation>) {
  for (const animation of animations.values()) animation.cancel();
  animations.clear();
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}
