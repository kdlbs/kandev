"use client";

import { useEffect } from "react";
import { useWebSocketClient } from "@/lib/ws/connection";

export function useSystemMetricsSubscription(enabled: boolean) {
  const client = useWebSocketClient();

  useEffect(() => {
    if (!enabled || !client) return;
    return client.subscribeSystemMetrics();
  }, [client, enabled]);
}
