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
      target = nextTarget;
      start = element.scrollTop;
      startedAt = time;
    }
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
  const onKey = (event: KeyboardEvent) => {
    if (SCROLL_KEYS.has(event.key)) interrupt();
  };
  const events = ["wheel", "touchstart", "pointerdown"] as const;
  for (const event of events) element.addEventListener(event, interrupt, { passive: true });
  element.addEventListener("keydown", onKey);
  const motion: ScrollMotion = {
    request: () => {
      if (!disposed && frame === null && canFollow()) frame = requestAnimationFrame(tick);
    },
    cancel,
    isRunning: () => frame !== null,
    dispose: () => {
      disposed = true;
      cancel();
      for (const event of events) element.removeEventListener(event, interrupt);
      element.removeEventListener("keydown", onKey);
      if (activeMotions.get(element) === motion) activeMotions.delete(element);
    },
  };
  activeMotions.set(element, motion);
  return motion;
}
