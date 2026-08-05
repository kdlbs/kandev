import { afterEach, describe, expect, it, vi } from "vitest";
import { TASK_ROW_DOM_ATTR, revealSidebarTask, taskRowSelector } from "./task-navigation";

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

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("taskRowSelector", () => {
  it("escapes task ids for use in a CSS selector", () => {
    expect(taskRowSelector("task:a")).toBe(`[${TASK_ROW_DOM_ATTR}="task\\:a"]`);
  });
});

describe("revealSidebarTask", () => {
  it("scrolls an off-screen rendered row with nearest alignment", async () => {
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: 120, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(row.scrollIntoView).toHaveBeenCalledWith({ block: "nearest", inline: "nearest" });
  });

  it("does not reposition a row that is already inside the viewport", async () => {
    const viewport = mountViewport();
    const row = mountRow(viewport, TEST_TASK_ID, { x: 0, y: 20, width: 320, height: 24 });

    await expect(revealSidebarTask(TEST_TASK_ID, (callback) => callback())).resolves.toBe(true);
    expect(row.scrollIntoView).not.toHaveBeenCalled();
  });

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
