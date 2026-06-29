"use client";

import { useTaskDeletedToast } from "@/hooks/use-task-deleted-toast";

/** Mounts the task-deleted toast hook inside the ToastProvider tree. */
export function TaskDeletedToastBridge() {
  useTaskDeletedToast();
  return null;
}
