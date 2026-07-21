"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/** Surfaces sidebar preference sync errors. Mount once inside the app's ToastProvider. */
export function useSidebarViewsSync() {
  const syncError = useAppStore((s) => s.sidebarViews.syncError);
  const taskPrefsSyncError = useAppStore((s) => s.sidebarTaskPrefs.syncError);
  const clearError = useAppStore((s) => s.clearSidebarSyncError);
  const clearTaskPrefsError = useAppStore((s) => s.clearSidebarTaskPrefsSyncError);
  const { toast } = useToast();

  useEffect(() => {
    if (!syncError) return;
    toast({ title: "Sidebar views", description: syncError, variant: "error" });
    clearError();
  }, [syncError, toast, clearError]);

  useEffect(() => {
    if (!taskPrefsSyncError) return;
    toast({ title: "Sidebar task preferences", description: taskPrefsSyncError, variant: "error" });
    clearTaskPrefsError();
  }, [taskPrefsSyncError, toast, clearTaskPrefsError]);
}
