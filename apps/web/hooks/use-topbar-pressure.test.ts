import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  resolveTopbarPressure,
  TOPBAR_TITLE_MIN_PX,
  useTopbarPressure,
  type TopbarPressureRefs,
} from "./use-topbar-pressure";

// A chain wide enough that the two thresholds are far apart.
const chain = { chainFull: 300, chainMin: 80, titleNatural: 200 };

describe("resolveTopbarPressure", () => {
  it("applies no pressure while the full chain and floored title fit", () => {
    expect(resolveTopbarPressure({ available: 400, ...chain })).toEqual({
      crumbsCollapsed: false,
      actionsOverflowed: false,
    });
  });

  it("collapses ancestry before actions overflow", () => {
    // Between the two thresholds: too tight for the full chain, fine for the
    // collapsed one. Ancestry yields; actions stay inline.
    expect(resolveTopbarPressure({ available: 300, ...chain })).toEqual({
      crumbsCollapsed: true,
      actionsOverflowed: false,
    });
  });

  it("overflows actions only when even the collapsed chain squeezes the title floor", () => {
    expect(resolveTopbarPressure({ available: 150, ...chain })).toEqual({
      crumbsCollapsed: true,
      actionsOverflowed: true,
    });
  });

  it("never overflows actions without also collapsing ancestry", () => {
    const pressure = resolveTopbarPressure({
      available: 0,
      chainFull: 10,
      chainMin: 10,
      titleNatural: 10,
    });
    expect(pressure.actionsOverflowed).toBe(true);
    expect(pressure.crumbsCollapsed).toBe(true);
  });

  it("reserves only the floor for long titles", () => {
    // chainFull + natural title would not fit, but the policy only defends the
    // floor, and chainFull + floor fits exactly.
    expect(resolveTopbarPressure({ available: 300 + TOPBAR_TITLE_MIN_PX, ...chain })).toEqual({
      crumbsCollapsed: false,
      actionsOverflowed: false,
    });
  });

  it("reserves the natural width of titles shorter than the floor", () => {
    expect(
      resolveTopbarPressure({ available: 100, chainFull: 60, chainMin: 60, titleNatural: 50 }),
    ).toEqual({ crumbsCollapsed: true, actionsOverflowed: true });
  });
});

// --- Hook-level coverage -----------------------------------------------------
//
// The reducer above is pure and easy. What breaks silently is the wiring around
// it: which elements are observed, which mutations count as a re-measure, and
// the anti-oscillation subtraction. jsdom has no layout, so widths are stubbed
// and the observers are driven by hand.

const NO_PRESSURE = { crumbsCollapsed: false, actionsOverflowed: false };

class StubResizeObserver {
  static instances: StubResizeObserver[] = [];
  readonly observed = new Set<Element>();
  private readonly callback: () => void;

  constructor(callback: () => void) {
    this.callback = callback;
    StubResizeObserver.instances.push(this);
  }

  observe(target: Element) {
    this.observed.add(target);
  }

  unobserve(target: Element) {
    this.observed.delete(target);
  }

  disconnect() {
    this.observed.clear();
  }

  /** Stands in for the browser noticing a box changed size. */
  emit() {
    this.callback();
  }
}

function setWidth(element: HTMLElement, width: number) {
  for (const property of ["offsetWidth", "clientWidth"]) {
    Object.defineProperty(element, property, { value: width, configurable: true });
  }
}

/** A ghost measurement child: an outer box whose width the hook reads, holding
 *  the `data-label` span that a rename mutates in place. */
function ghostChild(width: number, label: string): HTMLElement {
  const box = document.createElement("span");
  setWidth(box, width);
  const text = document.createElement("span");
  text.setAttribute("data-label", label);
  box.append(text);
  return box;
}

type Harness = {
  refs: TopbarPressureRefs;
  leadZone: HTMLElement;
  leading: HTMLElement;
  ghost: HTMLElement;
  rightZone: HTMLElement;
};

/**
 * Lead zone laid out as the real bar is: a shrink-0 sibling whose width is
 * subtracted, plus the breadcrumb itself, which is not.
 */
function buildHarness({
  leadZoneWidth,
  leadingWidth,
  chainFull,
  chainMin,
  titleNatural,
  rightZoneWidth,
}: {
  leadZoneWidth: number;
  leadingWidth: number;
  chainFull: number;
  chainMin: number;
  titleNatural: number;
  rightZoneWidth: number;
}): Harness {
  const leadZone = document.createElement("div");
  setWidth(leadZone, leadZoneWidth);

  const leading = document.createElement("div");
  setWidth(leading, leadingWidth);

  const breadcrumb = document.createElement("nav");
  breadcrumb.dataset.slot = "breadcrumb";
  setWidth(breadcrumb, 1);

  leadZone.append(leading, breadcrumb);

  const ghost = document.createElement("div");
  ghost.append(
    ghostChild(chainFull, "Workspace"),
    ghostChild(chainMin, "Workspace"),
    ghostChild(titleNatural, "Task title"),
  );

  const rightZone = document.createElement("div");
  setWidth(rightZone, rightZoneWidth);

  document.body.append(leadZone, ghost, rightZone);

  return {
    refs: {
      leadZone: { current: leadZone },
      ghost: { current: ghost },
      rightZone: { current: rightZone },
    },
    leadZone,
    leading,
    ghost,
    rightZone,
  };
}

/** Lets jsdom deliver queued MutationObserver records. */
async function flushObservers() {
  await act(async () => {
    await Promise.resolve();
  });
}

/** Roomy enough that nothing folds until a test makes it tight. */
function unpressuredHarness(): Harness {
  return buildHarness({
    leadZoneWidth: 500,
    leadingWidth: 100,
    chainFull: 300,
    chainMin: 80,
    titleNatural: 200,
    rightZoneWidth: 120,
  });
}

let originalResizeObserver: typeof globalThis.ResizeObserver | undefined;

function stubResizeObserver() {
  originalResizeObserver = globalThis.ResizeObserver;
  StubResizeObserver.instances = [];
  globalThis.ResizeObserver = StubResizeObserver as unknown as typeof globalThis.ResizeObserver;
}

function restoreResizeObserver() {
  cleanup();
  globalThis.ResizeObserver = originalResizeObserver as typeof globalThis.ResizeObserver;
  document.body.replaceChildren();
}

describe("useTopbarPressure re-measure triggers", () => {
  beforeEach(stubResizeObserver);
  afterEach(restoreResizeObserver);

  it("re-measures when a crumb label is renamed in place", async () => {
    // available = 500 - 100 = 400; chainFull 300 + floor 96 fits.
    const harness = unpressuredHarness();
    const { result } = renderHook(() => useTopbarPressure(harness.refs, true));
    expect(result.current).toEqual(NO_PRESSURE);

    // A rename changes only a `data-label` attribute: no child is added or
    // removed, and no text node changes. Without `attributeFilter:
    // ["data-label"]` on the ghost observer this widening goes unnoticed.
    const expandedChain = harness.ghost.children[0] as HTMLElement;
    setWidth(expandedChain, 320);
    await act(async () => {
      expandedChain.firstElementChild?.setAttribute("data-label", "A much longer project name");
    });
    await flushObservers();

    // 400 < 320 + 96, so ancestry collapses; the collapsed chain still fits.
    expect(result.current).toEqual({ crumbsCollapsed: true, actionsOverflowed: false });
  });

  it("observes the lead zone's measured children, not just the container", () => {
    const harness = unpressuredHarness();
    renderHook(() => useTopbarPressure(harness.refs, true));

    const observer = StubResizeObserver.instances[0];
    // `measureAvailable` subtracts this sibling's width, so its resizes have to
    // be a trigger too; the container's own width can stay constant through one.
    expect(observer.observed.has(harness.leading)).toBe(true);
    expect(observer.observed.has(harness.leadZone)).toBe(true);
    expect(observer.observed.has(harness.rightZone)).toBe(true);
  });

  it("re-measures when a left action mounts without the container resizing", async () => {
    const harness = unpressuredHarness();
    const { result } = renderHook(() => useTopbarPressure(harness.refs, true));
    expect(result.current).toEqual(NO_PRESSURE);

    // The lead zone is exactly as wide as before; only its contents changed.
    const leftAction = document.createElement("div");
    setWidth(leftAction, 200);
    await act(async () => {
      harness.leadZone.append(leftAction);
    });
    await flushObservers();

    // available = 500 - 100 - 200 = 200, which is under chainFull + floor.
    expect(result.current).toEqual({ crumbsCollapsed: true, actionsOverflowed: false });
    expect(StubResizeObserver.instances[0].observed.has(leftAction)).toBe(true);
  });
});

describe("useTopbarPressure state", () => {
  beforeEach(stubResizeObserver);
  afterEach(restoreResizeObserver);

  it("does not oscillate when overflowing the actions frees up lead-zone width", async () => {
    // available = 250 - 100 = 150, under chainMin 80 + floor 96 = 176.
    const harness = buildHarness({
      leadZoneWidth: 250,
      leadingWidth: 100,
      chainFull: 300,
      chainMin: 80,
      titleNatural: 200,
      rightZoneWidth: 200,
    });
    let renders = 0;
    const { result } = renderHook(() => {
      renders += 1;
      return useTopbarPressure(harness.refs, true);
    });
    expect(result.current).toEqual({ crumbsCollapsed: true, actionsOverflowed: true });

    const rendersAfterOverflow = renders;

    // Folding the actions into the "…" menu shrinks the right zone by 160 and
    // hands that width to the lead zone. Measured naively, available becomes
    // 310 and the actions would unfold, re-widening the right zone, and so on.
    setWidth(harness.rightZone, 40);
    setWidth(harness.leadZone, 410);
    await act(async () => {
      StubResizeObserver.instances[0].emit();
    });

    expect(result.current).toEqual({ crumbsCollapsed: true, actionsOverflowed: true });
    // Unchanged state must not re-render; a flip-flop would show up here.
    expect(renders).toBe(rendersAfterOverflow);

    // Real extra room still unfolds them: this is hysteresis, not a latch.
    // available = 610 - 100 - 160 freed = 350, over 176 but under 300 + 96.
    setWidth(harness.leadZone, 610);
    await act(async () => {
      StubResizeObserver.instances[0].emit();
    });
    expect(result.current).toEqual({ crumbsCollapsed: true, actionsOverflowed: false });
  });

  it("reports no pressure while disabled", () => {
    const harness = buildHarness({
      leadZoneWidth: 100,
      leadingWidth: 90,
      chainFull: 300,
      chainMin: 80,
      titleNatural: 200,
      rightZoneWidth: 200,
    });
    const { result } = renderHook(() => useTopbarPressure(harness.refs, false));

    expect(result.current).toEqual(NO_PRESSURE);
    expect(StubResizeObserver.instances).toHaveLength(0);
  });

  it("reports no pressure where ResizeObserver is unavailable", () => {
    const harness = buildHarness({
      leadZoneWidth: 100,
      leadingWidth: 90,
      chainFull: 300,
      chainMin: 80,
      titleNatural: 200,
      rightZoneWidth: 200,
    });
    // Test DOMs without layout render the unpressured bar rather than a bar
    // whose every zero-width measurement reads as maximum pressure.
    globalThis.ResizeObserver = undefined as unknown as typeof globalThis.ResizeObserver;

    const { result } = renderHook(() => useTopbarPressure(harness.refs, true));

    expect(result.current).toEqual(NO_PRESSURE);
  });
});
