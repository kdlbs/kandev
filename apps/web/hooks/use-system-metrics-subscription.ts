"use client";

import { useEffect } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

export function useSystemMetricsSubscription(enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    const client = getWebSocketClient();
    if (!client) return;
    return client.subscribeSystemMetrics();
  }, [enabled]);
}
