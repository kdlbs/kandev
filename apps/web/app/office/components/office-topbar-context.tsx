"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import type { ParentCrumb } from "@/components/page-topbar";

/**
 * Chrome a detail page contributes to the office topbar: the same data the
 * PageTopbar contract takes as props. The trail is data (title, parents);
 * interactive controls travel through the contract's action slots instead of
 * being injected as markup.
 */
export type OfficeTopbarChrome = {
  title: string;
  titleSlot?: ReactNode;
  icon?: ReactNode;
  parents?: ParentCrumb[];
  leftActions?: ReactNode;
  actions?: ReactNode;
};

const OfficeTopbarValueContext = createContext<OfficeTopbarChrome | null>(null);
const OfficeTopbarSetterContext = createContext<
  ((chrome: OfficeTopbarChrome | null) => void) | null
>(null);

export function OfficeTopbarChromeProvider({ children }: { children: ReactNode }) {
  const [chrome, setChrome] = useState<OfficeTopbarChrome | null>(null);
  return (
    <OfficeTopbarSetterContext.Provider value={setChrome}>
      <OfficeTopbarValueContext.Provider value={chrome}>
        {children}
      </OfficeTopbarValueContext.Provider>
    </OfficeTopbarSetterContext.Provider>
  );
}

/** Read side: the office shell merges this into its PageShell props. */
export function useOfficeTopbarChrome(): OfficeTopbarChrome | null {
  return useContext(OfficeTopbarValueContext);
}

/**
 * Write side: a detail page declares its topbar chrome; `null` declares none
 * (e.g. while the record is still loading).
 */
export function useOfficeTopbar(chrome: OfficeTopbarChrome | null): void {
  const setChrome = useContext(OfficeTopbarSetterContext);
  // Deliberately no dep array: slot elements close over page state (save
  // handlers, toggle callbacks), so every render re-registers fresh nodes.
  // Pages do not consume the value context, so this cannot loop; only the
  // shell's topbar re-renders.
  useEffect(() => {
    setChrome?.(chrome);
  });
  useEffect(() => {
    return () => setChrome?.(null);
  }, [setChrome]);
}
