"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Separator } from "@kandev/ui/separator";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { WorkspaceUnansweredInboxCard } from "@/components/settings/workspaces/workspace-unanswered-inbox-card";
import type { WorkspaceState } from "@/lib/state/slices";
import {
  buildUnansweredInboxUpdates,
  isValidRetentionDays,
  unansweredInboxSavedFrom,
  type UnansweredInboxSaved,
} from "./workspace-unanswered-inbox-draft";

type Workspace = WorkspaceState["items"][number];

function useUnansweredInboxDraft(workspace: Workspace) {
  const [saved, setSaved] = useState<UnansweredInboxSaved>(() =>
    unansweredInboxSavedFrom(workspace),
  );
  const [enabled, setEnabled] = useState(saved.enabled);
  const [retentionDays, setRetentionDays] = useState(String(saved.retentionDays));
  const retentionValid = isValidRetentionDays(retentionDays);
  const enabledIsDirty = enabled !== saved.enabled;
  const retentionIsDirty = !retentionValid || Number(retentionDays) !== saved.retentionDays;
  const discard = () => {
    setEnabled(saved.enabled);
    setRetentionDays(String(saved.retentionDays));
  };
  return {
    saved,
    setSaved,
    enabled,
    setEnabled,
    retentionDays,
    setRetentionDays,
    retentionValid,
    enabledIsDirty,
    retentionIsDirty,
    discard,
  };
}

/**
 * Owns the unanswered-question inbox settings for one workspace, saved
 * through the shared settings bar as its own contributor.
 */
export function WorkspaceUnansweredInboxSettings({
  workspace,
  canManage,
}: {
  workspace: Workspace;
  canManage: boolean;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const storeApi = useAppStoreApi();
  const draft = useUnansweredInboxDraft(workspace);

  const save = async () => {
    const submitted = { enabled: draft.enabled, retentionDays: draft.retentionDays };
    const updates = buildUnansweredInboxUpdates(submitted, draft.saved);
    try {
      const updated = await updateWorkspaceAction(workspace.id, updates);
      const nextSaved = unansweredInboxSavedFrom({ ...workspace, ...updated });
      draft.setSaved(nextSaved);
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(
        workspaces.items.map((item) =>
          item.id === workspace.id
            ? {
                ...item,
                clarification_unanswered_inbox_enabled: nextSaved.enabled,
                clarification_unanswered_retention_days: nextSaved.retentionDays,
              }
            : item,
        ),
      );
    } catch (error) {
      toast({
        title: t("workspaces:failedToSaveWorkspace"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
      throw error;
    }
  };

  useSettingsSaveContributor({
    id: `workspace:${workspace.id}:unanswered-inbox`,
    revision: JSON.stringify({ enabled: draft.enabled, retentionDays: draft.retentionDays }),
    isDirty: draft.enabledIsDirty || draft.retentionIsDirty,
    canSave: draft.retentionValid,
    invalidReason: draft.retentionValid ? undefined : t("workspaces:unansweredRetentionInvalid"),
    save,
    discard: draft.discard,
  });

  const handleEnabledChange = (next: boolean) => {
    if (!next && !draft.retentionValid) draft.setRetentionDays(String(draft.saved.retentionDays));
    draft.setEnabled(next);
  };

  return (
    <>
      <WorkspaceUnansweredInboxCard
        canManage={canManage}
        enabled={draft.enabled}
        retentionDays={draft.retentionDays}
        retentionValid={draft.retentionValid}
        enabledIsDirty={draft.enabledIsDirty}
        retentionIsDirty={draft.retentionIsDirty}
        onEnabledChange={handleEnabledChange}
        onRetentionChange={draft.setRetentionDays}
      />
      <Separator />
    </>
  );
}
