/** Bind browser pinch input to the scroll region of a single rendered diagram. */
export function bindMermaidPinch(region: HTMLElement, zoomBy: (factor: number) => void) {
  const controller = new AbortController();
  const options = { passive: false, signal: controller.signal };
  let gestureScale: number | null = null;
  let touching = false;
  const trackTouches = (event: TouchEvent) => {
    touching = event.touches.length > 0;
  };
  for (const type of ["touchstart", "touchend", "touchcancel"] as const) {
    region.addEventListener(type, trackTouches, { passive: true, signal: controller.signal });
  }

  region.addEventListener(
    "wheel",
    (event) => {
      if (!event.ctrlKey || touching || !region.querySelector("svg")) return;
      event.preventDefault();
      if (gestureScale !== null) return;
      const unit = event.deltaMode === 1 ? 16 : 1;
      const pixels = event.deltaY * (event.deltaMode === 2 ? region.clientHeight : unit);
      zoomBy(Math.exp(-Math.max(-400, Math.min(400, pixels)) / 100));
    },
    options,
  );

  region.addEventListener(
    "gesturestart",
    (event) => {
      if (touching || !region.querySelector("svg")) return;
      event.preventDefault();
      gestureScale = 1;
    },
    options,
  );
  region.addEventListener(
    "gesturechange",
    (event) => {
      if (gestureScale === null) return;
      event.preventDefault();
      const { scale } = event as Event & { scale: number };
      if (!Number.isFinite(scale) || scale <= 0) return;
      zoomBy(scale / gestureScale);
      gestureScale = scale;
    },
    options,
  );
  region.addEventListener(
    "gestureend",
    (event) => {
      if (gestureScale === null) return;
      event.preventDefault();
      gestureScale = null;
    },
    options,
  );
  return () => controller.abort();
}
