"use client";

import { useContext } from "react";
import { QuickTerminalContext } from "@/components/quick-terminal/quick-terminal-provider";

/** Opens the shared ephemeral host-shell dialog. */
export function useQuickTerminalLauncher() {
  const openQuickTerminal = useContext(QuickTerminalContext);
  if (!openQuickTerminal) {
    throw new Error("useQuickTerminalLauncher must be used within QuickTerminalProvider");
  }
  return openQuickTerminal;
}
