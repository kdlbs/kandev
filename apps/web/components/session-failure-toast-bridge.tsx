"use client";

import { useSessionFailureToast } from "@/hooks/use-session-failure-toast";

/** Mounts the session failure toast hook inside the ToastProvider tree. */
export function SessionFailureToastBridge() {
  useSessionFailureToast();
  return null;
}
