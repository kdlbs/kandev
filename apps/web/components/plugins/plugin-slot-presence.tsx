"use client";

import { createContext, useLayoutEffect, useRef, type ReactNode } from "react";

export const ObservedPluginSlotContext = createContext<string | null>(null);

/** Reports owners with mounted content, including asynchronous null/content changes. */
export function PluginSlotPresence({
  name,
  children,
  onChange,
}: {
  name: string;
  children: ReactNode;
  onChange: (pluginIds: string[]) => void;
}) {
  const container = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const element = container.current;
    if (!element) return;
    let previous: string[] | undefined;
    const update = () => {
      const owners = new Set<string>();
      for (const registration of element.querySelectorAll<HTMLElement>(
        "[data-plugin-slot-owner]",
      )) {
        if (registration.childNodes.length > 0 && registration.dataset.pluginSlotOwner) {
          owners.add(registration.dataset.pluginSlotOwner);
        }
      }
      const next = [...owners];
      if (previous?.length === next.length && previous.every((id, index) => id === next[index])) {
        return;
      }
      previous = next;
      onChange(next);
    };
    update();
    const observer = new MutationObserver(update);
    observer.observe(element, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [name, onChange]);

  return (
    <ObservedPluginSlotContext.Provider value={name}>
      <div ref={container} className="contents">
        {children}
      </div>
    </ObservedPluginSlotContext.Provider>
  );
}
