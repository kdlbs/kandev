import { useEffect, useRef, useState, type PointerEvent } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

// i18n-exempt: CSS media query, not displayed copy.
const HOVER_QUERY = "(hover: hover)";

function hasOpenSidebarSurface(root: HTMLElement | null): boolean {
  if (!root) return false;
  if (root.querySelector('[data-slot="context-menu-trigger"][data-state="open"]')) return true;
  return Array.from(root.querySelectorAll('[aria-expanded="true"][aria-controls]')).some(
    (trigger) => {
      const surface = document.getElementById(trigger.getAttribute("aria-controls") ?? "");
      return surface !== null && !root.contains(surface);
    },
  );
}

function observeSidebarSurfaces(root: HTMLElement | null, onChange: () => void) {
  const observer = new MutationObserver(onChange);
  if (root)
    observer.observe(root, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ["aria-expanded", "data-state"],
    });
  return observer;
}

function returnFocusToToggle(root: HTMLElement | null) {
  requestAnimationFrame(() => {
    if (root?.isConnected) root.querySelector<HTMLButtonElement>("[data-sidebar-toggle]")?.focus();
  });
}

type SidebarHoverOptions = {
  enabled: boolean;
  delayMs: number;
  collapsed: boolean;
  pathname: string | null;
};

export function useSidebarHoverReveal({
  collapsed,
  pathname,
  enabled,
  delayMs,
}: SidebarHoverOptions) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const eligible = enabled && !isMobile && isFinePointer;
  const ref = useRef<HTMLElement>(null);
  const [revealed, setRevealed] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inside = useRef(false);
  const focused = useRef(false);
  const mounted = useRef(true);
  const previousSettings = useRef({ enabled, delayMs, eligible });

  function cancelTimer() {
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = null;
  }

  function dismiss() {
    cancelTimer();
    setRevealed(false);
  }

  function closeIfOutside() {
    if (
      mounted.current &&
      !inside.current &&
      !focused.current &&
      !hasOpenSidebarSurface(ref.current)
    ) {
      setRevealed(false);
    }
  }

  useEffect(() => {
    mounted.current = true;
    const media = window.matchMedia(HOVER_QUERY);
    const reset = (capabilityChanged = false) => {
      cancelTimer();
      const changed =
        capabilityChanged ||
        previousSettings.current.eligible !== eligible ||
        previousSettings.current.enabled !== enabled ||
        previousSettings.current.delayMs !== delayMs;
      if (changed && collapsed && focused.current) returnFocusToToggle(ref.current);
      previousSettings.current = { enabled, delayMs, eligible };
      focused.current = false;
      setRevealed(false);
    };
    reset();
    const onCapabilityChange = () => reset(true);
    media.addEventListener("change", onCapabilityChange);
    return () => {
      mounted.current = false;
      cancelTimer();
      media.removeEventListener("change", onCapabilityChange);
    };
    // The callbacks only read refs; saved-state and route changes reset the interaction.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collapsed, pathname, eligible, delayMs]);

  useEffect(() => {
    if (!revealed) return;
    const observer = observeSidebarSurfaces(ref.current, closeIfOutside);

    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape" || event.defaultPrevented || hasOpenSidebarSurface(ref.current))
        return;
      event.preventDefault();
      setRevealed(false);
      if (focused.current) returnFocusToToggle(ref.current);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => {
      observer.disconnect();
      window.removeEventListener("keydown", onKeyDown);
    };
    // Only the reveal state changes this subscription; containment reads live refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revealed]);

  const handlers = {
    onPointerEnter(event: PointerEvent<HTMLElement>) {
      if (inside.current || event.pointerType === "touch") return;
      inside.current = true;
      if (!collapsed || !eligible || !window.matchMedia(HOVER_QUERY).matches) return;
      timer.current = setTimeout(() => {
        timer.current = null;
        setRevealed(true);
      }, delayMs);
    },
    onPointerLeave() {
      inside.current = false;
      cancelTimer();
      closeIfOutside();
    },
    onFocusCapture() {
      focused.current = true;
    },
    onBlurCapture() {
      focused.current = false;
      // React focus events bubble through portals; wait for the destination's focus event.
      void Promise.resolve().then(closeIfOutside);
    },
  };

  return { ref, revealed: collapsed && revealed, dismiss, handlers };
}
