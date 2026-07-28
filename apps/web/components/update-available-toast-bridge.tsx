"use client";

import { useUpdateAvailableToast } from "@/hooks/use-update-available-toast";

/** Mounts the update-available toast hook inside the ToastProvider tree. */
export function UpdateAvailableToastBridge() {
  useUpdateAvailableToast();
  return null;
}
