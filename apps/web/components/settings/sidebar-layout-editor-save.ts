"use client";

import { useCallback, useEffect, useRef, useState, type MutableRefObject } from "react";
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
  requestGenerationsRef: MutableRefObject<Map<string, number>>;
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
  requestGenerationsRef,
  onOperationError,
}: SaveContext) {
  const submittedWorkspaceId = workspaceRef.current;
  if (!submittedWorkspaceId) return;
  const submittedGeneration = generationsRef.current.get(submittedWorkspaceId) ?? 0;
  const submittedRequestGeneration =
    (requestGenerationsRef.current.get(submittedWorkspaceId) ?? 0) + 1;
  requestGenerationsRef.current.set(submittedWorkspaceId, submittedRequestGeneration);
  const submittedRevision = savedRef.current.revision;
  const submitted = { ...draftRef.current, revision: submittedRevision };
  const response = await updateUserSettings({
    sidebar_layout_state: {
      workspace_id: submittedWorkspaceId,
      expected_revision: submittedRevision,
      layout: toApiSidebarLayout(submitted),
    },
  });
  const latest = response.settings.sidebar_layouts_by_workspace?.[submittedWorkspaceId];
  const next = latest
    ? fromApiSidebarLayout(latest)
    : { ...submitted, revision: submittedRevision + 1 };
  if (requestGenerationsRef.current.get(submittedWorkspaceId) !== submittedRequestGeneration) {
    return;
  }
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
  if (workspaceRef.current === submittedWorkspaceId) {
    const current = store.getState().userSettings;
    setUserSettings(mapUserSettingsResponse(response, current));
    onOperationError(null);
  }
}

function useLatestSidebarLayout({
  workspaceId,
  catalog,
  acknowledge,
  setUserSettings,
  store,
  savedRef,
  workspaceRef,
  draftRef,
  setStatus,
}: {
  workspaceId: string | null;
  catalog: ShortcutCatalogEntry[];
  acknowledge: (workspaceId: string, nextSaved: SidebarLayout, applyDraft: boolean) => void;
  setUserSettings: (settings: UserSettingsState) => void;
  store: { getState: () => { userSettings: UserSettingsState } };
  savedRef: MutableRefObject<SidebarLayout>;
  workspaceRef: MutableRefObject<string | null>;
  draftRef: MutableRefObject<SidebarLayout>;
  setStatus: (value: "error" | null) => void;
}) {
  const [latestLoading, setLatestLoading] = useState(false);
  const latestAttemptRef = useRef(0);

  useEffect(() => {
    latestAttemptRef.current += 1;
    setLatestLoading(false);
  }, [workspaceId]);

  const loadLatest = useCallback(async () => {
    if (!workspaceId) return;
    const requestedWorkspaceId = workspaceId;
    const requestedAttempt = ++latestAttemptRef.current;
    setLatestLoading(true);
    try {
      const response = await fetchUserSettings({ cache: "no-store" });
      const latest = response.settings.sidebar_layouts_by_workspace?.[requestedWorkspaceId];
      const next = latest ? fromApiSidebarLayout(latest) : defaultSidebarLayout();
      if (latestAttemptRef.current !== requestedAttempt) return;
      const recoverUnsupportedDraft =
        draftRef.current.unsupportedVersion === true && next.unsupportedVersion !== true;
      acknowledge(requestedWorkspaceId, next, recoverUnsupportedDraft);
      if (
        workspaceRef.current === requestedWorkspaceId &&
        latestAttemptRef.current === requestedAttempt
      ) {
        savedRef.current = materializeSidebarPluginNodes(next, catalog);
      }
      if (
        workspaceRef.current === requestedWorkspaceId &&
        latestAttemptRef.current === requestedAttempt
      ) {
        const current = store.getState().userSettings;
        setUserSettings(mapUserSettingsResponse(response, current));
        setStatus(null);
      }
    } catch {
      if (
        workspaceRef.current === requestedWorkspaceId &&
        latestAttemptRef.current === requestedAttempt
      ) {
        setStatus("error");
      }
    } finally {
      if (
        workspaceRef.current === requestedWorkspaceId &&
        latestAttemptRef.current === requestedAttempt
      ) {
        setLatestLoading(false);
      }
    }
  }, [
    acknowledge,
    catalog,
    draftRef,
    savedRef,
    setStatus,
    setUserSettings,
    store,
    workspaceId,
    workspaceRef,
  ]);

  return { latestLoading, loadLatest };
}

function discardSidebarLayout({
  draftRef,
  savedRef,
  setDraft,
  workspaceRef,
  generationsRef,
  requestGenerationsRef,
  setSaveError,
  onOperationError,
}: {
  draftRef: MutableRefObject<SidebarLayout>;
  savedRef: MutableRefObject<SidebarLayout>;
  setDraft: (next: SidebarLayout) => void;
  workspaceRef: MutableRefObject<string | null>;
  generationsRef: MutableRefObject<Map<string, number>>;
  requestGenerationsRef: MutableRefObject<Map<string, number>>;
  setSaveError: (value: "conflict" | "error" | null) => void;
  onOperationError: (value: string | null) => void;
}) {
  const currentWorkspaceId = workspaceRef.current;
  if (!currentWorkspaceId) return;
  generationsRef.current.set(
    currentWorkspaceId,
    (generationsRef.current.get(currentWorkspaceId) ?? 0) + 1,
  );
  requestGenerationsRef.current.set(
    currentWorkspaceId,
    (requestGenerationsRef.current.get(currentWorkspaceId) ?? 0) + 1,
  );
  draftRef.current = savedRef.current;
  setDraft(savedRef.current);
  setSaveError(null);
  onOperationError(null);
}

type SidebarLayoutSaveProps = {
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
  requestGenerationsRef: MutableRefObject<Map<string, number>>;
  onOperationError: (value: string | null) => void;
};

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
  requestGenerationsRef,
  onOperationError,
}: SidebarLayoutSaveProps) {
  const [saveError, setSaveError] = useState<"conflict" | "error" | null>(null);
  const saveAttemptRef = useRef(0);
  const saveWorkspaceRef = useRef(workspaceId);
  useEffect(() => {
    if (saveWorkspaceRef.current === workspaceId) return;
    saveWorkspaceRef.current = workspaceId;
    saveAttemptRef.current += 1;
    setSaveError(null);
  }, [workspaceId]);
  const { latestLoading, loadLatest } = useLatestSidebarLayout({
    workspaceId,
    catalog,
    acknowledge,
    setUserSettings,
    store,
    savedRef,
    workspaceRef,
    draftRef,
    setStatus: (value) => setSaveError(value),
  });
  useSettingsSaveContributor({
    id: "sidebar-layout",
    order: 20,
    revision: `${workspaceId ?? "none"}:${layoutValue(draft)}`,
    isDirty: Boolean(workspaceId && dirty),
    canSave: validationValid && !draft.unsupportedVersion,
    invalidReason,
    save: async () => {
      const requestedWorkspaceId = workspaceRef.current;
      const requestedAttempt = ++saveAttemptRef.current;
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
          requestGenerationsRef,
          onOperationError,
        });
        if (
          workspaceRef.current === requestedWorkspaceId &&
          saveAttemptRef.current === requestedAttempt
        ) {
          setSaveError(null);
        }
      } catch (error) {
        if (
          workspaceRef.current === requestedWorkspaceId &&
          saveAttemptRef.current === requestedAttempt
        ) {
          setSaveError(error instanceof ApiError && error.status === 409 ? "conflict" : "error");
        }
        throw error;
      }
    },
    discard: () =>
      discardSidebarLayout({
        draftRef,
        savedRef,
        setDraft,
        workspaceRef,
        generationsRef,
        requestGenerationsRef,
        setSaveError,
        onOperationError,
      }),
  });

  return { saveError, setSaveError, latestLoading, loadLatest };
}
