"use client";

import { createContext, useCallback, useRef, useState } from "react";
import dynamic from "@/lib/routing/client-dynamic";

export const QuickTerminalContext = createContext<(() => void) | null>(null);

const HostShellDialog = dynamic(
  () => import("@/components/settings/host-shell-dialog").then((module) => module.HostShellDialog),
  { ssr: false },
);

/** Owns the one ephemeral Quick Terminal surface shared by all launch points. */
export function QuickTerminalProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  const openQuickTerminal = useCallback(() => {
    previousFocusRef.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setOpen(true);
  }, []);

  const handleOpenChange = useCallback((nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) {
      requestAnimationFrame(() => {
        previousFocusRef.current?.focus();
        previousFocusRef.current = null;
      });
    }
  }, []);

  return (
    <QuickTerminalContext.Provider value={openQuickTerminal}>
      {children}
      {open && <HostShellDialog open={open} onOpenChange={handleOpenChange} presentation="quick" />}
    </QuickTerminalContext.Provider>
  );
}
