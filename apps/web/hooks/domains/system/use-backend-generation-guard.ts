"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchSystemInfo } from "@/lib/api/domains/system-api";
import { signalBackendReloadRequired } from "@/lib/platform/backend-reload-coordinator";
import { readBootPayload } from "@/src/boot-payload";

/**
 * Confirms that a recovered WebSocket belongs to the backend generation that
 * rendered this document. A failed or incomplete check is deliberately
 * inconclusive, so it cannot interrupt a usable page without proof.
 */
export function useBackendGenerationGuard(): void {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const pageBootIdRef = useRef(readBootPayload().runtime?.bootId);
  const previousStatusRef = useRef<typeof connectionStatus | null>(null);
  const requestSequenceRef = useRef(0);
  const mountedRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    if (connectionStatus !== "connected") {
      previousStatusRef.current = connectionStatus;
      requestSequenceRef.current += 1;
      return;
    }
    if (previousStatusRef.current === "connected") return;
    previousStatusRef.current = "connected";
    const requestSequence = ++requestSequenceRef.current;

    void fetchSystemInfo({ cache: "no-store" })
      .then((info) => {
        if (!mountedRef.current || requestSequence !== requestSequenceRef.current) return;
        const pageBootId = pageBootIdRef.current?.trim();
        const liveBootId = info.boot_id?.trim();
        if (!pageBootId || !liveBootId || pageBootId === liveBootId) return;
        signalBackendReloadRequired("boot_id_changed");
      })
      .catch(() => {
        // A failed request does not prove that the backend generation changed.
      });

    return () => {
      mountedRef.current = false;
    };
  }, [connectionStatus]);
}
