"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/** Watches for session failure notifications and shows an error toast. Mount once inside ToastProvider. */
export function useSessionFailureToast() {
  const notification = useAppStore((s) => s.sessionFailureNotification);
  const clearNotification = useAppStore((s) => s.setSessionFailureNotification);
  const { toast } = useToast();
  const shownRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!notification) return;
    if (shownRef.current.has(notification.sessionId)) {
      clearNotification(null);
      return;
    }
    shownRef.current.add(notification.sessionId);
    toast({
      title: "Task failed to start",
      description: notification.message,
      variant: "error",
    });
    clearNotification(null);
  }, [notification, toast, clearNotification]);
}
