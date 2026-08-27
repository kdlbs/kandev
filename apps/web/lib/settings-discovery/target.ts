export const SETTINGS_TARGET_ATTRIBUTE = "data-settings-target";
export const SETTINGS_TARGET_FOCUS_ATTRIBUTE = "data-settings-target-focus";
export const SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE = "data-settings-target-highlight";
export const SETTINGS_TARGET_REQUEST_EVENT = "kandev:settings-target";

export type SettingsTargetRequestDetail = { targetId: string };

export type SettingsTargetRegistry = {
  register: (targetId: string, element: HTMLElement) => () => void;
  request: (targetId: string) => boolean;
};

type RevealOptions = {
  highlightDurationMs?: number;
  reducedMotion?: boolean;
  settleDurationMs?: number;
};

const DEFAULT_HIGHLIGHT_DURATION_MS = 1400;
const DEFAULT_SETTLE_DURATION_MS = 2500;
const highlightTimers = new WeakMap<HTMLElement, number>();

export function settingsTargetFromHash(hash: string): string | null {
  if (!hash.startsWith("#") || hash.length === 1) return null;
  try {
    return decodeURIComponent(hash.slice(1)) || null;
  } catch {
    return null;
  }
}

export function settingsTargetSelector(targetId: string): string {
  return `[${SETTINGS_TARGET_ATTRIBUTE}="${CSS.escape(targetId)}"]`;
}

export function emitSettingsTargetRequest(targetId: string): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<SettingsTargetRequestDetail>(SETTINGS_TARGET_REQUEST_EVENT, {
      detail: { targetId },
    }),
  );
}

export function createSettingsTargetRegistry(
  reveal: (element: HTMLElement) => void = revealSettingsTarget,
): SettingsTargetRegistry {
  const targets = new Map<string, HTMLElement>();
  let pendingTargetId: string | null = null;

  return {
    register(targetId, element) {
      targets.set(targetId, element);
      element.setAttribute(SETTINGS_TARGET_ATTRIBUTE, targetId);
      if (pendingTargetId === targetId) {
        pendingTargetId = null;
        reveal(element);
      }
      return () => {
        if (targets.get(targetId) === element) targets.delete(targetId);
        if (element.getAttribute(SETTINGS_TARGET_ATTRIBUTE) === targetId) {
          element.removeAttribute(SETTINGS_TARGET_ATTRIBUTE);
        }
      };
    },
    request(targetId) {
      const element = targets.get(targetId);
      if (!element) {
        pendingTargetId = targetId;
        return false;
      }
      pendingTargetId = null;
      reveal(element);
      return true;
    },
  };
}

export function revealSettingsTarget(element: HTMLElement, options: RevealOptions = {}): void {
  const reducedMotion = options.reducedMotion ?? prefersReducedMotion();
  element.scrollIntoView?.({
    behavior: reducedMotion ? "auto" : "smooth",
    block: "center",
  });

  focusTargetWithin(element);
  restartTargetHighlight(element, options.highlightDurationMs ?? DEFAULT_HIGHLIGHT_DURATION_MS);
  keepTargetCentered(element, options.settleDurationMs ?? DEFAULT_SETTLE_DURATION_MS);
}

let cancelActiveSettle: (() => void) | null = null;

/**
 * Content around the target often loads asynchronously after the initial
 * scroll (e.g. sections above it fetch data and grow), pushing the target out
 * of view. Re-center it whenever an ancestor resizes during a short settle
 * window, backing off as soon as the user scrolls or interacts.
 */
function keepTargetCentered(element: HTMLElement, durationMs: number): void {
  cancelActiveSettle?.();
  if (typeof ResizeObserver === "undefined" || durationMs <= 0) return;

  const baselines = new Map<Element, string>();
  const observer = new ResizeObserver((entries) => {
    let shifted = false;
    for (const entry of entries) {
      const size = `${entry.contentRect.width}x${entry.contentRect.height}`;
      if (baselines.has(entry.target) && baselines.get(entry.target) !== size) shifted = true;
      baselines.set(entry.target, size);
    }
    if (shifted && element.isConnected) {
      element.scrollIntoView?.({ behavior: "auto", block: "center" });
    }
  });
  for (let node: HTMLElement | null = element; node; node = node.parentElement) {
    observer.observe(node);
  }

  const interactionEvents = ["wheel", "touchstart", "pointerdown", "keydown"] as const;
  const cancel = () => {
    observer.disconnect();
    window.clearTimeout(timer);
    for (const type of interactionEvents) {
      window.removeEventListener(type, cancel, { capture: true });
    }
    if (cancelActiveSettle === cancel) cancelActiveSettle = null;
  };
  for (const type of interactionEvents) {
    window.addEventListener(type, cancel, { capture: true, passive: true });
  }
  const timer = window.setTimeout(cancel, durationMs);
  cancelActiveSettle = cancel;
}

function focusTargetWithin(element: HTMLElement): void {
  const focusTarget =
    element.querySelector<HTMLElement>(`[${SETTINGS_TARGET_FOCUS_ATTRIBUTE}]`) ??
    element.querySelector<HTMLElement>(
      `input:not([disabled]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])`,
    );
  const target = focusTarget ?? element;
  if (!focusTarget && !element.hasAttribute("tabindex")) element.tabIndex = -1;
  target.focus({ preventScroll: true });
}

function restartTargetHighlight(element: HTMLElement, durationMs: number): void {
  const previousTimer = highlightTimers.get(element);
  if (previousTimer !== undefined) window.clearTimeout(previousTimer);
  element.removeAttribute(SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE);
  void element.offsetWidth;
  element.setAttribute(SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE, "true");
  const timer = window.setTimeout(() => {
    element.removeAttribute(SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE);
    highlightTimers.delete(element);
  }, durationMs);
  highlightTimers.set(element, timer);
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}
