"use client";

import { createContext, memo, useContext, type ReactNode } from "react";
import { useMobileTerminals } from "@/hooks/domains/session/use-mobile-terminals";

type MobileTerminalsContextValue = ReturnType<typeof useMobileTerminals>;

const MobileTerminalsContext = createContext<MobileTerminalsContextValue | null>(null);

/**
 * Provider that owns the single `useMobileTerminals` instance for a mobile
 * terminal pane. Both the chip-style picker pill and the picker sheet's list
 * read state from this context instead of calling `useMobileTerminals`
 * themselves — three separate `useState`-backed lists used to drift out of
 * sync after `addTerminal` / `removeTerminal`, leaving the pane showing the
 * wrong active terminal until a server WS push reconciled them.
 */
export const MobileTerminalsProvider = memo(function MobileTerminalsProvider({
  sessionId,
  children,
}: {
  sessionId: string | null;
  children: ReactNode;
}) {
  const value = useMobileTerminals(sessionId);
  return (
    <MobileTerminalsContext.Provider value={value}>{children}</MobileTerminalsContext.Provider>
  );
});

export function useMobileTerminalsContext(): MobileTerminalsContextValue {
  const ctx = useContext(MobileTerminalsContext);
  if (!ctx) {
    throw new Error("useMobileTerminalsContext must be used inside a <MobileTerminalsProvider>");
  }
  return ctx;
}
