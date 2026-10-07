import { describe, expect, it } from "vitest";
import type { CollisionDetection } from "@dnd-kit/core";
import { createSidebarListCollisionDetection } from "./sidebar-reorder-collision";

function setRect(element: HTMLElement, rect: DOMRect): void {
  element.getBoundingClientRect = () => rect;
}

function createCollisionArgs(
  pointerCoordinates: { x: number; y: number } | null,
  listRect = new DOMRect(0, 0, 120, 120),
  itemRect = new DOMRect(0, 0, 120, 32),
) {
  const list = document.createElement("div");
  list.setAttribute("data-sidebar-reorder-list", "");
  setRect(list, listRect);
  const item = document.createElement("div");
  list.append(item);
  const id = "rule-1";
  const droppable = {
    id,
    node: { current: item },
  };
  return {
    list,
    args: {
      active: { id },
      collisionRect: itemRect,
      droppableRects: new Map([[id, itemRect]]),
      droppableContainers: [droppable],
      pointerCoordinates,
    } as unknown as Parameters<CollisionDetection>[0],
  };
}

describe("sidebarListCollisionDetection", () => {
  it("uses the nearest rule in gaps inside the visible list", () => {
    const { args } = createCollisionArgs({ x: 60, y: 80 });
    const collision = createSidebarListCollisionDetection();

    expect(collision.detect(args).map((item) => item.id)).toEqual(["rule-1"]);
  });

  it("rejects a pointer outside the list instead of choosing the nearest rule", () => {
    const { args } = createCollisionArgs({ x: 150, y: 16 });
    const collision = createSidebarListCollisionDetection();

    expect(collision.detect(args)).toEqual([]);
  });

  it("rejects a pointer clipped by the visible scroll region", () => {
    const { args, list } = createCollisionArgs({ x: 16, y: 80 });
    const collision = createSidebarListCollisionDetection();
    const scrollRegion = document.createElement("div");
    scrollRegion.style.overflowY = "auto";
    Object.defineProperty(scrollRegion, "clientHeight", { value: 48 });
    setRect(scrollRegion, new DOMRect(0, 0, 120, 48));
    scrollRegion.append(list);
    document.body.append(scrollRegion);

    try {
      expect(collision.detect(args)).toEqual([]);
    } finally {
      scrollRegion.remove();
    }
  });

  it("keeps keyboard sortable movement on the nearest rule", () => {
    const { args } = createCollisionArgs(null);
    const collision = createSidebarListCollisionDetection();

    expect(collision.detect(args).map((item) => item.id)).toEqual(["rule-1"]);
  });

  it("rejects collision rectangles outside the list when pointer coordinates are absent", () => {
    const { args } = createCollisionArgs(
      null,
      new DOMRect(0, 0, 120, 120),
      new DOMRect(140, 0, 32, 32),
    );
    const collision = createSidebarListCollisionDetection();

    expect(collision.detect(args)).toEqual([]);
  });

  it("retains the starting bounds while scrolling moves the list under the drag", () => {
    const { args, list } = createCollisionArgs({ x: 60, y: 8 });
    const collision = createSidebarListCollisionDetection();
    expect(collision.detect(args)).toHaveLength(1);

    setRect(list, new DOMRect(0, 28, 120, 120));
    expect(collision.detect(args)).toHaveLength(1);

    collision.reset();
    expect(collision.detect(args)).toEqual([]);
  });

  it("rejects a drop in starting bounds after the visible list moves down", () => {
    const { args, list } = createCollisionArgs({ x: 60, y: 8 });
    const collision = createSidebarListCollisionDetection();
    document.body.append(list);
    try {
      expect(collision.detect(args)).toHaveLength(1);

      setRect(list, new DOMRect(0, 28, 120, 120));
      expect(collision.detect(args)).toHaveLength(1);
      expect(collision.isDropWithinCurrentVisibleBounds()).toBe(false);

      args.pointerCoordinates = { x: 60, y: 32 };
      collision.detect(args);
      expect(collision.isDropWithinCurrentVisibleBounds()).toBe(true);
    } finally {
      list.remove();
    }
  });
});
