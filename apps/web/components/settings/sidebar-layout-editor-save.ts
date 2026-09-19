"use client";

import { useCallback, useState, type MutableRefObject } from "react";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { ApiError } from "@/lib/api/client";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import {
  defaultSidebarLayout,
  fromApiSidebarLayout,
  toApiSidebarLayout,
  type SidebarLayout,
} from "@/lib/sidebar/layout-types";
import { materializeSidebarPluginNodes } from "@/lib/sidebar/layout-projection";
import { layoutValue } from "./sidebar-layout-editor-state";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";

type SaveContext = {
  catalog: ShortcutCatalogEntry[];
  acknowledge: (workspaceId: string, nextSaved: SidebarLayout, applyDraft: boolean) => void;
  setUserSettings: (settings: UserSettingsState) => void;
  store: { getState: () => { userSettings: UserSettingsState } };
  draftRef: MutableRefObject<SidebarLayout>;
  savedRef: MutableRefObject<SidebarLayout>;
  workspaceRef: MutableRefObject<string | null>;
  generationsRef: MutableRefObject<Map<string, number>>;
  onOperationError: (value: string | null) => void;
};

export async function submitSidebarLayout({
  catalog,
  acknowledge,
  setUserSettings,
  store,
  draftRef,
  savedRef,
  workspaceRef,
  generationsRef,
  onOperationError,
}: SaveContext) {
  const submittedWorkspaceId = workspaceRef.current;
  if (!submittedWorkspaceId) return;
  const submittedGeneration = generationsRef.current.get(submittedWorkspaceId) ?? 0;
  const submitted = { ...draftRef.current, revision: savedRef.current.revision };
  const response = await updateUserSettings({
    sidebar_layout_state: {
      workspace_id: submittedWorkspaceId,
      expected_revision: savedRef.current.revision,
      layout: toApiSidebarLayout(submitted),
    },
  });
  const latest = response.settings.sidebar_layouts_by_workspace?.[submittedWorkspaceId];
  const next = latest
    ? fromApiSidebarLayout(latest)
    : { ...submitted, revision: savedRef.current.revision + 1 };
  const stillCurrent =
    workspaceRef.current === submittedWorkspaceId &&
    (generationsRef.current.get(submittedWorkspaceId) ?? 0) === submittedGeneration &&
    layoutValue(draftRef.current) === layoutValue(submitted);
  acknowledge(submittedWorkspaceId, next, stillCurrent);
  if (workspaceRef.current === submittedWorkspaceId) {
    savedRef.current = materializeSidebarPluginNodes(next, catalog);
    if (stillCurrent) {
      draftRef.current = savedRef.current;
    }
  }
  const current = store.getState().userSettings;
  setUserSettings(mapUserSettingsResponse(response, current));
  onOperationError(null);
}

export function useSidebarLayoutSave({
  workspaceId,
  draft,
  dirty,
  validationValid,
  invalidReason,
  catalog,
  setDraft,
  acknowledge,
  setUserSettings,
  store,
  draftRef,
  savedRef,
  workspaceRef,
  generationsRef,
  onOperationError,
}: {
  workspaceId: string | null;
  draft: SidebarLayout;
  dirty: boolean;
  validationValid: boolean;
  invalidReason?: string;
  catalog: ShortcutCatalogEntry[];
  setDraft: (next: SidebarLayout | ((current: SidebarLayout) => SidebarLayout)) => void;
  acknowledge: (workspaceId: string, nextSaved: SidebarLayout, applyDraft: boolean) => void;
  setUserSettings: (settings: UserSettingsState) => void;
  store: { getState: () => { userSettings: UserSettingsState } };
  draftRef: MutableRefObject<SidebarLayout>;
  savedRef: MutableRefObject<SidebarLayout>;
  workspaceRef: MutableRefObject<string | null>;
  generationsRef: MutableRefObject<Map<string, number>>;
  onOperationError: (value: string | null) => void;
}) {
  const [saveError, setSaveError] = useState<"conflict" | "error" | null>(null);
  const [latestLoading, setLatestLoading] = useState(false);

  const loadLatest = useCallback(async () => {
    if (!workspaceId) return;
    const requestedWorkspaceId = workspaceId;
    setLatestLoading(true);
    try {
      const response = await fetchUserSettings({ cache: "no-store" });
      const latest = response.settings.sidebar_layouts_by_workspace?.[requestedWorkspaceId];
      const next = latest ? fromApiSidebarLayout(latest) : defaultSidebarLayout();
      acknowledge(requestedWorkspaceId, next, false);
      if (workspaceRef.current === requestedWorkspaceId) {
        savedRef.current = materializeSidebarPluginNodes(next, catalog);
      }
      const current = store.getState().userSettings;
      setUserSettings(mapUserSettingsResponse(response, current));
      setSaveError(null);
    } catch {
      setSaveError("error");
    } finally {
      setLatestLoading(false);
    }
  }, [acknowledge, catalog, savedRef, setUserSettings, store, workspaceId, workspaceRef]);

  useSettingsSaveContributor({
    id: "sidebar-layout",
    order: 20,
    revision: `${workspaceId ?? "none"}:${layoutValue(draft)}`,
    isDirty: Boolean(workspaceId && dirty),
    canSave: validationValid,
    invalidReason,
    save: async () => {
      try {
        await submitSidebarLayout({
          catalog,
          acknowledge,
          setUserSettings,
          store,
          draftRef,
          savedRef,
          workspaceRef,
          generationsRef,
          onOperationError,
        });
        setSaveError(null);
      } catch (error) {
        setSaveError(error instanceof ApiError && error.status === 409 ? "conflict" : "error");
        throw error;
      }
    },
    discard: () => {
      const currentWorkspaceId = workspaceRef.current;
      if (!currentWorkspaceId) return;
      generationsRef.current.set(
        currentWorkspaceId,
        (generationsRef.current.get(currentWorkspaceId) ?? 0) + 1,
      );
      draftRef.current = savedRef.current;
      setDraft(savedRef.current);
      setSaveError(null);
      onOperationError(null);
    },
  });

  return { saveError, setSaveError, latestLoading, loadLatest };
}
