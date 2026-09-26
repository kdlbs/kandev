"use client";

import { useEffect, useRef, useState, type RefObject } from "react";

export type TopbarPressure = {
  /** Ancestry renders in its ellipsis form at every width, not just below `md`. */
  crumbsCollapsed: boolean;
  /** Designated overflow actions fold into the trailing menu. */
  actionsOverflowed: boolean;
};

export type TopbarPressureInput = {
  /** Width the breadcrumb can occupy without pushing its siblings. */
  available: number;
  /** Natural width of the fully expanded ancestry chain. */
  chainFull: number;
  /** Natural width of the collapsed chain (ellipsis plus last parent). */
  chainMin: number;
  /** Natural width of the untruncated title cluster. */
  titleNatural: number;
};

/** The title never yields below this; ancestry and actions fold first. */
export const TOPBAR_TITLE_MIN_PX = 96;

const NO_PRESSURE: TopbarPressure = { crumbsCollapsed: false, actionsOverflowed: false };

/**
 * The one space policy for page chrome: ancestry collapses into the ellipsis
 * dropdown first, the title then truncates down to its floor, and designated
 * actions overflow last. `chainFull >= chainMin` makes that order a property
 * of the thresholds rather than something each caller has to sequence.
 */
export function resolveTopbarPressure(input: TopbarPressureInput): TopbarPressure {
  const titleFloor = Math.min(input.titleNatural, TOPBAR_TITLE_MIN_PX);
  const actionsOverflowed = input.available < input.chainMin + titleFloor;
  const crumbsCollapsed = actionsOverflowed || input.available < input.chainFull + titleFloor;
  return { crumbsCollapsed, actionsOverflowed };
}

export type TopbarPressureRefs = {
  /** Zone holding the leading content, the breadcrumb, and leftActions. */
  leadZone: RefObject<HTMLDivElement | null>;
  /** Invisible measurement row: children are [chainFull, chainMin, title]. */
  ghost: RefObject<HTMLDivElement | null>;
  /** Trailing zone (actions plus the status trigger). */
  rightZone: RefObject<HTMLDivElement | null>;
};

function gapTotal(el: HTMLElement, boxes: number): number {
  if (boxes < 2) return 0;
  const gap = Number.parseFloat(getComputedStyle(el).columnGap || "0");
  return Number.isNaN(gap) ? 0 : gap * (boxes - 1);
}

/**
 * Space left for the breadcrumb: the lead zone's width minus its shrink-0
 * siblings and the gaps between rendered children. `display: none` children
 * produce no flex box and no gap, so zero-width children are skipped.
 */
function measureAvailable(leadZone: HTMLElement): number {
  let siblings = 0;
  let boxes = 0;
  for (const child of leadZone.children) {
    const el = child as HTMLElement;
    if (el.offsetWidth === 0) continue;
    boxes += 1;
    if (el.dataset.slot !== "breadcrumb") siblings += el.offsetWidth;
  }
  return leadZone.clientWidth - siblings - gapTotal(leadZone, boxes);
}

/**
 * Measures the topbar and resolves the space policy. Reads natural widths from
 * the ghost row (stable regardless of what is currently folded) and, while
 * actions are overflowed, subtracts the width they freed so the decision keeps
 * comparing against the unfolded layout instead of oscillating.
 */
export function useTopbarPressure(refs: TopbarPressureRefs, enabled: boolean): TopbarPressure {
  const [pressure, setPressure] = useState(NO_PRESSURE);
  const stateRef = useRef(NO_PRESSURE);
  const rightFullRef = useRef(0);

  useEffect(() => {
    if (!enabled) {
      stateRef.current = NO_PRESSURE;
      setPressure(NO_PRESSURE);
      return;
    }
    const leadZone = refs.leadZone.current;
    const ghost = refs.ghost.current;
    // Test DOMs without ResizeObserver (or layout) render the unpressured bar.
    if (!leadZone || !ghost || typeof ResizeObserver === "undefined") return;

    const measure = () => {
      const [chainFullEl, chainMinEl, titleEl] = Array.from(ghost.children) as HTMLElement[];
      const rightNow = refs.rightZone.current?.offsetWidth ?? 0;
      if (!stateRef.current.actionsOverflowed) rightFullRef.current = rightNow;
      const freed = stateRef.current.actionsOverflowed
        ? Math.max(0, rightFullRef.current - rightNow)
        : 0;
      const next = resolveTopbarPressure({
        available: measureAvailable(leadZone) - freed,
        chainFull: chainFullEl?.offsetWidth ?? 0,
        chainMin: chainMinEl?.offsetWidth ?? 0,
        titleNatural: titleEl?.offsetWidth ?? 0,
      });
      if (
        next.crumbsCollapsed !== stateRef.current.crumbsCollapsed ||
        next.actionsOverflowed !== stateRef.current.actionsOverflowed
      ) {
        stateRef.current = next;
        setPressure(next);
      }
    };

    const resizeObserver = new ResizeObserver(measure);
    // The lead zone's own width is not enough. `measureAvailable` subtracts the
    // width of each shrink-0 sibling, and a left action can appear, disappear,
    // or change width while the container stays exactly as wide — a resize the
    // container alone never reports. Observing the measured children is what
    // makes the subtraction and the trigger cover the same set of boxes.
    const observeChildren = () => {
      for (const child of leadZone.children) resizeObserver.observe(child as HTMLElement);
    };
    resizeObserver.observe(leadZone);
    observeChildren();
    if (refs.rightZone.current) resizeObserver.observe(refs.rightZone.current);
    // Ghost content changes when titles or crumb labels change. Labels render
    // as CSS generated content from data-label, so attribute mutations are the
    // signal for in-place renames; childList covers crumbs appearing/leaving.
    const mutationObserver = new MutationObserver(measure);
    mutationObserver.observe(ghost, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["data-label"],
    });
    // A left action that mounts later is a new box to observe, not just a new
    // width. Re-observing is idempotent, so the whole child list is re-armed.
    const leadZoneObserver = new MutationObserver(() => {
      observeChildren();
      measure();
    });
    leadZoneObserver.observe(leadZone, { childList: true });
    measure();
    return () => {
      resizeObserver.disconnect();
      mutationObserver.disconnect();
      leadZoneObserver.disconnect();
    };
  }, [refs.leadZone, refs.ghost, refs.rightZone, enabled]);

  return pressure;
}
