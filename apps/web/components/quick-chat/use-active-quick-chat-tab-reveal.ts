import { useLayoutEffect, useRef } from "react";

export function useActiveQuickChatTabReveal(activeIdentity: string | null) {
  const stripRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const strip = stripRef.current;
    const tab = strip?.querySelector<HTMLElement>("[data-quick-chat-active]");
    if (!strip || !tab || !activeIdentity) return;

    const stripBounds = strip.getBoundingClientRect();
    const scale = strip.offsetWidth ? stripBounds.width / strip.offsetWidth : 1;
    if (!scale || !strip.clientWidth) return;
    const left = stripBounds.left + strip.clientLeft * scale;
    const right = left + strip.clientWidth * scale;
    let bounds = tab.getBoundingClientRect();
    if (bounds.width > right - left) {
      bounds = tab.querySelector<HTMLElement>("button")?.getBoundingClientRect() ?? bounds;
    }
    const delta =
      bounds.width > right - left || bounds.left < left
        ? bounds.left - left
        : Math.max(0, bounds.right - right);
    if (delta) strip.scrollLeft += delta / scale;
  }, [activeIdentity]);

  return stripRef;
}
