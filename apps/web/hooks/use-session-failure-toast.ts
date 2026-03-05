"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/**
 * Watches for session failure notifications in the store and shows an error toast.
 * Mount once in a global layout component inside ToastProvider.
 */
export function useSessionFailureToast() {
  const notification = useAppStore((s) => s.sessionFailureNotification);
  const clearNotification = useAppStore((s) => s.setSessionFailureNotification);
  const { toast } = useToast();

  useEffect(() => {
    if (!notification) return;
    toast({
      title: "Task failed to start",
      description: notification.message,
      variant: "error",
    });
    clearNotification(null);
  }, [notification, toast, clearNotification]);
}
