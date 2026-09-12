"use client";

import { useNeedsYouInboxController } from "@/hooks/domains/needs-you-inbox/use-needs-you-inbox-controller";

/**
 * Mounts the Needs-you Inbox count slice's refresh triggers and snooze timer
 * at the app shell, so they run regardless of route (AC .34, AC .41). Renders
 * nothing; mirrors WebSocketConnector and the other app-shell bridges.
 */
export function NeedsYouInboxBridge() {
  useNeedsYouInboxController();
  return null;
}
