export type ScrollMotion = {
  request: () => void;
  cancel: () => void;
  dispose: () => void;
  isRunning: () => boolean;
};

const activeMotions = new WeakMap<HTMLElement, ScrollMotion>();
const SCROLL_KEYS = new Set(["ArrowUp", "ArrowDown", "PageUp", "PageDown", "Home", "End", " "]);

export function cancelChatScrollMotion(element: HTMLElement): void {
  activeMotions.get(element)?.cancel();
}

function listenForScrollIntent(element: HTMLElement, interrupt: () => void): () => void {
  const onKey = (event: KeyboardEvent) => {
    if (SCROLL_KEYS.has(event.key)) interrupt();
  };
  const onPointer = (event: PointerEvent) => {
    if (event.target !== element || element.offsetWidth <= element.clientWidth) return;
    const bounds = element.getBoundingClientRect();
    const left = bounds.left + element.clientLeft;
    if (event.clientX < left || event.clientX >= left + element.clientWidth) interrupt();
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
    if (touchY === undefined || y === undefined || Math.abs(y - touchY) < 6) return;
    touchY = undefined;
    interrupt();
  };
  element.addEventListener("wheel", interrupt, { passive: true });
  element.addEventListener("pointerdown", onPointer, { passive: true });
  element.addEventListener("touchstart", onTouchStart, { passive: true });
  element.addEventListener("touchmove", onTouchMove, { passive: true });
  element.addEventListener("touchend", onTouchEnd);
  element.addEventListener("touchcancel", onTouchEnd);
  element.addEventListener("keydown", onKey);
  return () => {
    element.removeEventListener("wheel", interrupt);
    element.removeEventListener("pointerdown", onPointer);
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
