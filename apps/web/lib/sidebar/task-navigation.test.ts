import { afterEach, describe, expect, it, vi } from "vitest";
import {
  TASK_ROW_DOM_ATTR,
  TASK_ROW_REVEAL_CLASS,
  cancelSidebarTaskReveal,
  revealSidebarTask,
  taskRowSelector,
} from "./task-navigation";

type Rect = { x: number; y: number; width: number; height: number };
const TEST_TASK_ID = "task-1";

function setRect(element: HTMLElement, rect: Rect) {
  vi.spyOn(element, "getBoundingClientRect").mockReturnValue({
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    top: rect.y,
    right: rect.x + rect.width,
    bottom: rect.y + rect.height,
    left: rect.x,
    toJSON: () => ({}),
  } as DOMRect);
}

function mountViewport(visible = true) {
  const viewport = document.createElement("div");
  viewport.dataset.testid = "task-sidebar-scroll";
  if (!visible) viewport.style.display = "none";
  setRect(viewport, { x: 0, y: 0, width: 320, height: 100 });
  document.body.appendChild(viewport);
  return viewport;
}

function mountRow(viewport: HTMLElement, taskId: string, rect: Rect) {
  const row = document.createElement("div");
  row.setAttribute(TASK_ROW_DOM_ATTR, taskId);
  row.scrollIntoView = vi.fn();
  setRect(row, rect);
  viewport.appendChild(row);
  return row;
}

function setReducedMotion(reducedMotion: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({ matches: reducedMotion })),
  );
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("taskRowSelector", () => {
  it("escapes task ids for use in a CSS selector", () => {
    expect(taskRowSelector("task:a")).toBe(`[${TASK_ROW_DOM_ATTR}="task\\:a"]`);
  });
});

describe("revealSidebarTask", () => {
  it("smoothly scrolls and cues an off-screen rendered row", async () => {
    setReducedMotion(false);
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: 120, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(row.scrollIntoView).toHaveBeenCalledWith({
      behavior: "smooth",
      block: "nearest",
      inline: "nearest",
    });
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(true);
  });

  it("uses immediate nearest scrolling and a non-animated cue for reduced motion", async () => {
    setReducedMotion(true);
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: -24, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(row.scrollIntoView).toHaveBeenCalledWith({
      behavior: "auto",
      block: "nearest",
      inline: "nearest",
    });
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(true);
  });

  it("does not reposition a row that is already inside the viewport", async () => {
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: 20, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(row.scrollIntoView).not.toHaveBeenCalled();
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(true);
  });

  it("restarts the cue without letting stale cleanup clear the newer cue", async () => {
    vi.useFakeTimers();
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: 120, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    vi.advanceTimersByTime(1_000);

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    vi.advanceTimersByTime(400);
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(true);

    vi.advanceTimersByTime(999);
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(true);
    vi.advanceTimersByTime(1);
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(false);
  });
});

describe("revealSidebarTask edge cases", () => {
  it("finds a row that renders after a later animation frame", async () => {
    const viewport = mountViewport();
    const callbacks: Array<() => void> = [];
    const navigation = revealSidebarTask("late", (callback) => callbacks.push(callback));

    expect(callbacks).toHaveLength(1);
    const row = mountRow(viewport, "late", { x: 0, y: 120, width: 320, height: 24 });
    callbacks.shift()!();

    await expect(navigation).resolves.toBe(true);
    expect(row.scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("does not scroll a superseded task when its row renders late", async () => {
    const viewport = mountViewport();
    const firstCallbacks: Array<() => void> = [];
    const firstNavigation = revealSidebarTask("task-a", (callback) =>
      firstCallbacks.push(callback),
    );

    expect(firstCallbacks).toHaveLength(1);

    const secondRow = mountRow(viewport, "task-b", { x: 0, y: 120, width: 320, height: 24 });
    await expect(revealSidebarTask("task-b", (callback) => callback())).resolves.toBe(true);
    expect(secondRow.scrollIntoView).toHaveBeenCalledTimes(1);

    const firstRow = mountRow(viewport, "task-a", { x: 0, y: 120, width: 320, height: 24 });
    firstCallbacks.shift()!();

    await expect(firstNavigation).resolves.toBe(false);
    expect(firstRow.scrollIntoView).not.toHaveBeenCalled();
  });

  it("cancels a pending reveal before a newer route is ready", async () => {
    const viewport = mountViewport();
    const callbacks: Array<() => void> = [];
    const navigation = revealSidebarTask("pending", (callback) => callbacks.push(callback));

    cancelSidebarTaskReveal();
    const row = mountRow(viewport, "pending", { x: 0, y: 120, width: 320, height: 24 });
    callbacks.shift()!();

    await expect(navigation).resolves.toBe(false);
    expect(row.scrollIntoView).not.toHaveBeenCalled();
  });

  it("clears an applied cue when navigation is canceled", async () => {
    vi.useFakeTimers();
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: 120, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(true);

    cancelSidebarTaskReveal();

    expect(row.classList.contains(TASK_ROW_REVEAL_CLASS)).toBe(false);
  });

  it("ignores a matching row inside a hidden sidebar viewport", async () => {
    const hiddenViewport = mountViewport(false);
    const hiddenRow = mountRow(hiddenViewport, TEST_TASK_ID, {
      x: 0,
      y: 120,
      width: 320,
      height: 24,
    });
    const visibleViewport = mountViewport();
    const visibleRow = mountRow(visibleViewport, TEST_TASK_ID, {
      x: 0,
      y: 120,
      width: 320,
      height: 24,
    });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(hiddenRow.scrollIntoView).not.toHaveBeenCalled();
    expect(visibleRow.scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("gives up after its bounded frame budget when no row renders", async () => {
    mountViewport();
    const callbacks: Array<() => void> = [];
    const navigation = revealSidebarTask("missing", (callback) => callbacks.push(callback));

    let frames = 0;
    while (callbacks.length > 0 && frames < 100) {
      callbacks.shift()!();
      frames += 1;
    }

    await expect(navigation).resolves.toBe(false);
    expect(frames).toBeLessThan(100);
    expect(callbacks).toHaveLength(0);
  });
});
