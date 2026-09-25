export type ScrollMotion = {
  request: () => void;
  cancel: () => void;
  dispose: () => void;
  isRunning: () => boolean;
};

const activeMotions = new WeakMap<HTMLElement, ScrollMotion>();
const EARLIER_SCROLL_KEYS = new Set(["ArrowUp", "PageUp", "Home"]);
const SCROLL_INTENT_THRESHOLD_PX = 6;

export function cancelChatScrollMotion(element: HTMLElement): void {
  activeMotions.get(element)?.cancel();
}

export function listenForScrollIntent(element: HTMLElement, interrupt: () => void): () => void {
  const onKey = (event: KeyboardEvent) => {
    const target = event.target;
    if (
      event.defaultPrevented ||
      (target instanceof Element &&
        target !== element &&
        target.closest(
          'button, input, select, textarea, a[href], [role="button"], [contenteditable]:not([contenteditable="false"])',
        ))
    )
      return;
    if (EARLIER_SCROLL_KEYS.has(event.key) || (event.key === " " && event.shiftKey)) interrupt();
  };
  let scrollbarPointerId: number | null = null;
  let scrollbarPointerY: number | undefined;
  const clearScrollbarPointer = () => {
    scrollbarPointerId = null;
    scrollbarPointerY = undefined;
  };
  const onPointerDown = (event: PointerEvent) => {
    clearScrollbarPointer();
    if (event.target !== element || element.offsetWidth <= element.clientWidth) return;
    const bounds = element.getBoundingClientRect();
    const contentLeft = bounds.left + element.clientLeft;
    const contentRight = contentLeft + element.clientWidth;
    if (event.clientX >= contentLeft && event.clientX < contentRight) return;
    const maxScrollTop = Math.max(0, element.scrollHeight - element.clientHeight);
    if (maxScrollTop <= 0) return;
    const thumbHeight = (element.clientHeight * element.clientHeight) / element.scrollHeight;
    const thumbTravel = Math.max(0, element.clientHeight - thumbHeight);
    const thumbTop =
      bounds.top + element.clientTop + (element.scrollTop / maxScrollTop) * thumbTravel;
    if (event.clientY < thumbTop) {
      interrupt();
      return;
    }
    if (event.clientY <= thumbTop + thumbHeight) {
      scrollbarPointerId = event.pointerId;
      scrollbarPointerY = event.clientY;
    }
  };
  const onPointerMove = (event: PointerEvent) => {
    if (scrollbarPointerId !== event.pointerId || scrollbarPointerY === undefined) return;
    const movingUp = scrollbarPointerY - event.clientY >= SCROLL_INTENT_THRESHOLD_PX;
    scrollbarPointerY = event.clientY;
    if (movingUp) {
      clearScrollbarPointer();
      interrupt();
    }
  };
  const onPointerEnd = (event: PointerEvent) => {
    if (scrollbarPointerId === event.pointerId) clearScrollbarPointer();
  };
  let touchY: number | undefined;
  const onTouchStart = (event: TouchEvent) => {
    touchY = event.touches[0]?.clientY;
  };
  const onTouchEnd = () => {
    touchY = undefined;
  };
  const onTouchMove = (event: TouchEvent) => {
    const y = event.touches[0]?.clientY;
    if (touchY === undefined || y === undefined || y - touchY < SCROLL_INTENT_THRESHOLD_PX) return;
    touchY = undefined;
    interrupt();
  };
  const onWheel = (event: WheelEvent) => {
    if (!event.ctrlKey && event.deltaY < 0) interrupt();
  };
  element.addEventListener("wheel", onWheel, { passive: true });
  element.addEventListener("pointerdown", onPointerDown, { passive: true });
  element.addEventListener("pointermove", onPointerMove, { passive: true });
  element.ownerDocument.defaultView?.addEventListener("pointerup", onPointerEnd);
  element.ownerDocument.defaultView?.addEventListener("pointercancel", onPointerEnd);
  element.addEventListener("touchstart", onTouchStart, { passive: true });
  element.addEventListener("touchmove", onTouchMove, { passive: true });
  element.addEventListener("touchend", onTouchEnd);
  element.addEventListener("touchcancel", onTouchEnd);
  element.addEventListener("keydown", onKey);
  return () => {
    element.removeEventListener("wheel", onWheel);
    element.removeEventListener("pointerdown", onPointerDown);
    element.removeEventListener("pointermove", onPointerMove);
    element.ownerDocument.defaultView?.removeEventListener("pointerup", onPointerEnd);
    element.ownerDocument.defaultView?.removeEventListener("pointercancel", onPointerEnd);
    element.removeEventListener("touchstart", onTouchStart);
    element.removeEventListener("touchmove", onTouchMove);
    element.removeEventListener("touchend", onTouchEnd);
    element.removeEventListener("touchcancel", onTouchEnd);
    element.removeEventListener("keydown", onKey);
  };
}

/** Geometry is read in frames, never in the message commit that requests follow. */
export function createChatScrollMotion(
  element: HTMLElement,
  canFollow: () => boolean,
  onInterrupt: () => void,
): ScrollMotion {
  let frame: number | null = null;
  let target = -1;
  let start = 0;
  let startedAt = 0;
  let previousFrameAt = 0;
  let disposed = false;
  const cancel = () => {
    if (frame !== null) cancelAnimationFrame(frame);
    frame = null;
    target = -1;
  };
  const tick = (time: number) => {
    frame = null;
    if (!canFollow() || disposed) {
      target = -1;
      return;
    }
    const nextTarget = Math.max(0, element.scrollHeight - element.clientHeight);
    if (nextTarget !== target) {
      // Retarget from the previous frame so uninterrupted growth still advances.
      startedAt = target === -1 ? time : previousFrameAt;
      target = nextTarget;
      start = element.scrollTop;
    }
    previousFrameAt = time;
    const progress = Math.min(1, (time - startedAt) / 180);
    const next = start + (target - start) * (1 - (1 - progress) ** 3);
    element.scrollTop = Math.abs(target - next) < 0.5 ? target : next;
    if (element.scrollTop !== target) frame = requestAnimationFrame(tick);
    else target = -1;
  };
  const interrupt = () => {
    cancel();
    onInterrupt();
  };
  const removeListeners = listenForScrollIntent(element, interrupt);
  const motion: ScrollMotion = {
    request: () => {
      if (!disposed && frame === null && canFollow()) frame = requestAnimationFrame(tick);
    },
    cancel,
    isRunning: () => frame !== null,
    dispose: () => {
      disposed = true;
      cancel();
      removeListeners();
      if (activeMotions.get(element) === motion) activeMotions.delete(element);
    },
  };
  activeMotions.set(element, motion);
  return motion;
}
