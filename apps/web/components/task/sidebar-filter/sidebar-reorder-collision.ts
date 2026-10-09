import { closestCenter, type CollisionDetection } from "@dnd-kit/core";

const CLIPPING_OVERFLOW = new Set(["auto", "clip", "hidden", "scroll"]);
const LIST_SELECTOR = "[data-sidebar-reorder-list]";

function clips(value: string): boolean {
  return CLIPPING_OVERFLOW.has(value);
}

export function isPointWithinVisibleBounds(
  element: HTMLElement,
  point: { x: number; y: number },
  bounds: Pick<DOMRect, "left" | "right" | "top" | "bottom"> = element.getBoundingClientRect(),
): boolean {
  let { left, right, top, bottom } = bounds;

  for (let ancestor = element.parentElement; ancestor; ancestor = ancestor.parentElement) {
    const style = getComputedStyle(ancestor);
    const ancestorRect = ancestor.getBoundingClientRect();
    if (clips(style.overflowX)) {
      left = Math.max(left, ancestorRect.left + ancestor.clientLeft);
      right = Math.min(right, ancestorRect.left + ancestor.clientLeft + ancestor.clientWidth);
    }
    if (clips(style.overflowY)) {
      top = Math.max(top, ancestorRect.top + ancestor.clientTop);
      bottom = Math.min(bottom, ancestorRect.top + ancestor.clientTop + ancestor.clientHeight);
    }
  }

  left = Math.max(left, 0);
  top = Math.max(top, 0);
  right = Math.min(right, window.innerWidth);
  bottom = Math.min(bottom, window.innerHeight);
  return point.x >= left && point.x <= right && point.y >= top && point.y <= bottom;
}

function unionBounds(first: DOMRect, current: DOMRect) {
  return {
    left: Math.min(first.left, current.left),
    right: Math.max(first.right, current.right),
    top: Math.min(first.top, current.top),
    bottom: Math.max(first.bottom, current.bottom),
  };
}

export function createSidebarListCollisionDetection(): {
  detect: CollisionDetection;
  reset: () => void;
  isDropWithinCurrentVisibleBounds: () => boolean;
} {
  let activeId: string | number | null = null;
  let activeList: HTMLElement | null = null;
  let initialListBounds: DOMRect | null = null;
  let lastPoint: { x: number; y: number } | null = null;

  const reset = () => {
    activeId = null;
    activeList = null;
    initialListBounds = null;
    lastPoint = null;
  };

  const detect: CollisionDetection = (args) => {
    const activeNode = args.droppableContainers.find((container) => container.id === args.active.id)
      ?.node.current;
    const list = activeNode?.closest<HTMLElement>(LIST_SELECTOR);
    if (!list) return [];

    if (activeId !== args.active.id) {
      activeId = args.active.id;
      initialListBounds = list.getBoundingClientRect();
    }
    activeList = list;

    const bounds = unionBounds(
      initialListBounds ?? list.getBoundingClientRect(),
      list.getBoundingClientRect(),
    );
    const point = args.pointerCoordinates ?? {
      x: (args.collisionRect.left + args.collisionRect.right) / 2,
      y: (args.collisionRect.top + args.collisionRect.bottom) / 2,
    };
    lastPoint = point;
    if (!isPointWithinVisibleBounds(list, point, bounds)) return [];

    return closestCenter(args);
  };

  const isDropWithinCurrentVisibleBounds = () => {
    return (
      activeList !== null &&
      activeList.isConnected &&
      lastPoint !== null &&
      isPointWithinVisibleBounds(activeList, lastPoint)
    );
  };

  return { detect, reset, isDropWithinCurrentVisibleBounds };
}
