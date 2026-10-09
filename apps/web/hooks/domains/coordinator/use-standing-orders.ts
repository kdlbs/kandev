"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listStandingOrders, type StandingOrder } from "@/lib/api/domains/coordinator-api";

export type StandingOrdersLoadStatus = "loading" | "ready" | "error";

/**
 * A coordinator's active standing orders in order-number order. The response
 * of the last request sent wins; a failed reload keeps the list already
 * loaded, so `error` means there is nothing to show.
 */
export function useStandingOrders(
  workspaceId: string,
  coordinatorId: string,
  options?: { includeRetired?: boolean },
) {
  const includeRetired = options?.includeRetired === true;
  const [orders, setOrders] = useState<StandingOrder[]>([]);
  const [status, setStatus] = useState<StandingOrdersLoadStatus>("loading");
  const sequenceRef = useRef(0);
  const loadedRef = useRef(false);

  const reload = useCallback((): Promise<void> => {
    const sequence = ++sequenceRef.current;
    return listStandingOrders(workspaceId, coordinatorId, { includeRetired })
      .then((response) => {
        if (sequence !== sequenceRef.current) return;
        loadedRef.current = true;
        setOrders(response.orders);
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || loadedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, includeRetired]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  useEffect(() => {
    loadedRef.current = false;
    setOrders([]);
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [reload]);

  return { orders, status, reload, retry };
}
