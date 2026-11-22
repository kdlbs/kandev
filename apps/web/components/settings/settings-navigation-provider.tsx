"use client";

import { createContext, useContext, useLayoutEffect, useState, type ReactNode } from "react";
import { subscribeHistoryCommit } from "@/lib/routing/navigation-guard";
import { settingsReturnHref } from "@/lib/settings/settings-return-navigation";

const SettingsReturnContext = createContext<string | null>(null);

function locationHref(): string {
  return window.location.pathname + window.location.search + window.location.hash;
}

export function SettingsNavigationProvider({ children }: { children: ReactNode }) {
  const [returnHref, setReturnHref] = useState<string | null>(null);

  useLayoutEffect(() => {
    let previousHref = locationHref();
    const committed = (fromHref: string, toHref: string) => {
      previousHref = toHref;
      setReturnHref((current) => settingsReturnHref(current, fromHref, toHref));
    };
    const unsubscribe = subscribeHistoryCommit(committed);
    const traversed = () => committed(previousHref, locationHref());
    // Only guard-approved traversals can commit a departure.
    window.addEventListener("popstate", traversed);
    return () => {
      unsubscribe();
      window.removeEventListener("popstate", traversed);
    };
  }, []);

  return (
    <SettingsReturnContext.Provider value={returnHref}>{children}</SettingsReturnContext.Provider>
  );
}

export function useSettingsReturnHref(): string | null {
  return useContext(SettingsReturnContext);
}
