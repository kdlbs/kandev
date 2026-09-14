"use client";

import { useEffect, useState } from "react";
import { getOfficeConfigSyncConfig } from "@/lib/api/domains/office-config-sync-api";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";

/**
 * Lightweight existence check for AC-OFFICE-CONFIG-SYNC-006.6: while a config
 * sync source is configured for the workspace, the raw-git `clone`/`pull`
 * controls and the filesystem sync page's apply-incoming/apply-outgoing
 * controls must show themselves as unavailable rather than let the operator
 * hit the server's 409 refusal. This intentionally does not reuse
 * `useOfficeConfigSync` (which also owns form state and mutation handlers) —
 * these consumers only need the existence bit.
 */
export function useOfficeConfigSyncActive(workspaceId: string): boolean {
  const [active, setActive] = useState(false);

  useEffect(() => {
    if (!workspaceId) {
      setActive(false);
      return;
    }
    let cancelled = false;
    const check = () => {
      getOfficeConfigSyncConfig(workspaceId)
        .then((cfg) => {
          if (!cancelled) setActive(cfg !== null);
        })
        .catch(() => {
          /* transient failures leave the last known state; the next tick retries */
        });
    };
    check();
    const id = setInterval(check, INTEGRATION_STATUS_REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [workspaceId]);

  return active;
}
