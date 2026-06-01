"use client";

import { useEffect } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { OfficeEventType, OfficeEventPayload } from "@/lib/types/office-events";

type OfficeWsMessage = BackendMessage<OfficeEventType, OfficeEventPayload>;

/**
 * Subscribes to one or more office WS events and runs `handler` for each.
 *
 * The TQ-native replacement for the legacy Zustand `office.refetchTrigger`
 * mechanism: instead of routing every office event through a single store
 * field that components watched via `useOfficeRefetch`, a consumer that
 * isn't on `useQuery` (or needs a side effect beyond cache invalidation)
 * subscribes directly to the events it cares about. The handler ref is
 * read fresh on each event so callers don't need a stable callback.
 */
export function useOfficeWsEffect(
  events: OfficeEventType[],
  handler: (message: OfficeWsMessage) => void,
): void {
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;
    const unsubs = events.map((event) =>
      client.on(event, (message) => handler(message as OfficeWsMessage)),
    );
    return () => {
      for (const unsub of unsubs) unsub();
    };
    // `events` is a literal array per call site; spread its contents so a
    // fresh array identity each render doesn't re-subscribe. `handler` is
    // intentionally omitted — callers pass a closure that's stable enough
    // for the refetch use case, and re-subscribing on every render would
    // churn the WS handler set.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [events.join(","), handler]);
}
