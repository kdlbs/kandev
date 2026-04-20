"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { DEFAULT_VIEW_ID } from "@/lib/state/slices/ui/sidebar-view-builtins";

/**
 * Runs one-time migration of locally-stored sidebar views to the backend and
 * surfaces sync errors as toasts. Mount once inside the app's ToastProvider.
 */
export function useSidebarViewsSync() {
  const views = useAppStore((s) => s.sidebarViews.views);
  const syncError = useAppStore((s) => s.sidebarViews.syncError);
  const userSettingsLoaded = useAppStore((s) => s.userSettings.loaded);
  const migrate = useAppStore((s) => s.migrateLocalViewsToBackend);
  const clearError = useAppStore((s) => s.clearSidebarSyncError);
  const { toast } = useToast();
  const migratedRef = useRef(false);

  useEffect(() => {
    if (migratedRef.current) return;
    // If user settings hydrated from the server, the views in memory are the
    // server's truth (or have already been overlaid by the hydrator). Skip
    // migration — the backend already has them, and any future change will
    // PATCH through mutateViews.
    if (userSettingsLoaded) {
      migratedRef.current = true;
      return;
    }
    const hasCustomViews = views.some((v) => v.id !== DEFAULT_VIEW_ID);
    if (!hasCustomViews) return;
    migratedRef.current = true;
    migrate();
  }, [views, userSettingsLoaded, migrate]);

  useEffect(() => {
    if (!syncError) return;
    toast({ title: "Sidebar views", description: syncError, variant: "error" });
    clearError();
  }, [syncError, toast, clearError]);
}
