import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Separator } from "@kandev/ui/separator";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useAppStoreApi } from "@/components/state-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { WorkspaceWipReleaseCard } from "@/components/settings/workspaces/workspace-wip-release-card";
import { useToast } from "@/components/toast-provider";
import type { WorkspaceState } from "@/lib/state/slices";
import {
  buildWipReleaseUpdates,
  isValidGraceMinutes,
  wipReleaseSavedFrom,
  type WipReleaseSaved,
} from "./workspace-wip-release-draft";

type Workspace = WorkspaceState["items"][number];

type Props = {
  workspace: Workspace;
  canManage: boolean;
};

function useWipReleaseDraft(workspace: Workspace) {
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation();
  const [saved, setSaved] = useState<WipReleaseSaved>(() => wipReleaseSavedFrom(workspace));
  const [enabled, setEnabled] = useState(saved.enabled);
  const [graceMinutes, setGraceMinutes] = useState(String(saved.graceMinutes));
  const graceValid = isValidGraceMinutes(graceMinutes);
  const enabledIsDirty = enabled !== saved.enabled;
  const graceIsDirty = !graceValid || Number(graceMinutes) !== saved.graceMinutes;

  const save = async () => {
    try {
      const updates = buildWipReleaseUpdates({ enabled, graceMinutes }, saved);
      const updated = await updateWorkspaceAction(workspace.id, updates);
      // Only the saved baseline moves; an edit made while the request was in
      // flight stays in the draft and stays dirty against the new baseline.
      const next = wipReleaseSavedFrom(updated);
      setSaved(next);
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(
        workspaces.items.map((item) =>
          item.id === updated.id
            ? {
                ...item,
                wip_sessionless_release_enabled: next.enabled,
                wip_sessionless_release_grace_minutes: next.graceMinutes,
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

  const discard = () => {
    setEnabled(saved.enabled);
    setGraceMinutes(String(saved.graceMinutes));
  };

  useSettingsSaveContributor({
    id: `workspace-wip-release:${workspace.id}`,
    revision: JSON.stringify({ enabled, graceMinutes }),
    isDirty: enabledIsDirty || graceIsDirty,
    canSave: graceValid,
    invalidReason: graceValid ? undefined : t("workspaces:wipReleaseGraceMustBePositive"),
    save,
    discard,
  });

  const onEnabledChange = (next: boolean) => {
    if (!next && !graceValid) setGraceMinutes(String(saved.graceMinutes));
    setEnabled(next);
  };

  return {
    enabled,
    graceMinutes,
    graceValid,
    enabledIsDirty,
    graceIsDirty,
    onEnabledChange,
    setGraceMinutes,
  };
}

/** Opt-in release of WIP slots held by tasks that no longer have a live session. */
export function WorkspaceWipReleaseSection({ workspace, canManage }: Props) {
  const draft = useWipReleaseDraft(workspace);
  return (
    <>
      <WorkspaceWipReleaseCard
        canManage={canManage}
        enabled={draft.enabled}
        graceMinutes={draft.graceMinutes}
        graceValid={draft.graceValid}
        enabledIsDirty={draft.enabledIsDirty}
        graceIsDirty={draft.graceIsDirty}
        onEnabledChange={draft.onEnabledChange}
        onGraceChange={draft.setGraceMinutes}
      />
      <Separator />
    </>
  );
}
