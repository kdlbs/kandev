"use client";

import { useEffect } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { coordinatorIdFromPath } from "@/hooks/domains/coordinator/coordinator-path";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";

/** Resets the copilot slot whenever the path leaves the coordinator's own
 *  Needs you and Queue routes, or moves to another coordinator. */
export function CoordinatorCopilotResetBridge() {
  const pathname = usePathname();
  useEffect(() => {
    useCopilotStore.getState().keepOnlyFor(coordinatorIdFromPath(pathname));
  }, [pathname]);
  return null;
}
