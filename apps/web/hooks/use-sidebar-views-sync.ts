"use client";

import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/** Surfaces sidebar preference sync errors. Mount once inside the app's ToastProvider. */
export function useSidebarViewsSync() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const syncError = useAppStore((s) => selectSidebarViews(s, workspaceId).syncError);
  const taskPrefsSyncError = useAppStore((s) => s.sidebarTaskPrefs.syncError);
  const clearError = useAppStore((s) => s.clearSidebarSyncError);
  const clearTaskPrefsError = useAppStore((s) => s.clearSidebarTaskPrefsSyncError);
  const store = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation("sidebar");

  useEffect(() => {
    if (!syncError || !workspaceId) return;
    if (store.getState().workspaces.activeId !== workspaceId) return;
    toast({ title: t("sidebar:views"), description: syncError, variant: "error" });
    clearError(workspaceId);
  }, [syncError, workspaceId, t, toast, clearError, store]);

  useEffect(() => {
    if (!taskPrefsSyncError) return;
    toast({
      title: t("sidebar:taskPreferences"),
      description: taskPrefsSyncError,
      variant: "error",
    });
    clearTaskPrefsError();
  }, [taskPrefsSyncError, t, toast, clearTaskPrefsError]);
}
