let nextViewportId = 0;

export function bindMermaidFrameResize(
  grip: HTMLElement,
  viewport: HTMLElement,
  resetButton: HTMLButtonElement,
) {
  const controller = new AbortController();
  const options = { signal: controller.signal };
  let drag: { pointer: number; y: number; height: number; max: number; moved: boolean } | undefined;
  viewport.id ||= `mermaid-frame-${++nextViewportId}`;
  grip.setAttribute("aria-controls", viewport.id);
  const height = () => viewport.getBoundingClientRect().height;
  const maximum = () => Math.max(64, height(), window.innerHeight, viewport.scrollHeight);
  const sync = () => {
    grip.setAttribute("aria-valuemin", String(Math.min(64, Math.round(height()))));
    grip.setAttribute("aria-valuemax", String(Math.ceil(maximum())));
    grip.setAttribute("aria-valuenow", String(Math.round(height())));
  };
  const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(sync);
  observer?.observe(viewport);
  sync();

  const reset = () => {
    viewport.style.removeProperty("--mermaid-frame-height");
    resetButton.hidden = true;
    sync();
  };
  const resize = (value: number, max: number) => {
    viewport.style.setProperty("--mermaid-frame-height", `${Math.max(64, Math.min(max, value))}px`);
    resetButton.hidden = true;
    sync();
  };
  resetButton.addEventListener("click", reset, options);
  grip.addEventListener("dblclick", reset, options);
  grip.addEventListener(
    "keydown",
    (event) => {
      if (event.key === "ArrowDown") resize(height() + 16, maximum());
      else if (event.key === "ArrowUp") resize(height() - 16, maximum());
      else if (event.key === "Enter") reset();
      else return;
      event.preventDefault();
      event.stopPropagation();
    },
    options,
  );
  grip.addEventListener(
    "pointerdown",
    (event) => {
      if (event.button !== 0 || !event.isPrimary) return;
      event.preventDefault();
      grip.focus({ preventScroll: true });
      grip.setPointerCapture(event.pointerId);
      drag = {
        pointer: event.pointerId,
        y: event.clientY,
        height: height(),
        max: maximum(),
        moved: false,
      };
    },
    options,
  );
  grip.addEventListener(
    "pointermove",
    (event) => {
      if (!drag || event.pointerId !== drag.pointer) return;
      const delta = event.clientY - drag.y;
      if (!drag.moved && Math.abs(delta) < 3) return;
      drag.moved = true;
      resize(drag.height + delta, drag.max);
    },
    options,
  );
  grip.addEventListener(
    "pointerup",
    (event) => {
      if (drag?.pointer === event.pointerId && !drag.moved && event.pointerType === "touch") {
        resetButton.hidden = !viewport.style.getPropertyValue("--mermaid-frame-height");
      }
    },
    options,
  );
  grip.addEventListener(
    "lostpointercapture",
    () => {
      drag = undefined;
    },
    options,
  );
  return () => {
    if (drag && grip.hasPointerCapture(drag.pointer)) grip.releasePointerCapture(drag.pointer);
    observer?.disconnect();
    controller.abort();
    resetButton.hidden = true;
  };
}
