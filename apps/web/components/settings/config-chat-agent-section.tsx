"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { SettingsCard } from "./settings-card";
import { UtilityAgentProfilePicker } from "./utility-agent-profile-picker";
import { SettingsCardHeader } from "./settings-card-header";
import { SettingsFieldLabel } from "./settings-typography";

export function ConfigChatAgentSection() {
  const { t } = useTranslation();
  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === s.workspaces.activeId) ?? null,
  );
  const profiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const currentProfileId = workspace?.default_config_agent_profile_id ?? "";
  const workspaceId = workspace?.id ?? null;
  const [syncedWorkspaceId, setSyncedWorkspaceId] = useState(workspaceId);
  const [savedProfileId, setSavedProfileId] = useState(currentProfileId);
  const [draftProfileId, setDraftProfileId] = useState(currentProfileId);

  const storeApi = useAppStoreApi();
  const isDirty = draftProfileId !== savedProfileId;

  useEffect(() => {
    if (workspaceId !== syncedWorkspaceId) {
      setSyncedWorkspaceId(workspaceId);
      setSavedProfileId(currentProfileId);
      setDraftProfileId(currentProfileId);
      return;
    }
    if (isDirty) return;
    setSavedProfileId(currentProfileId);
    setDraftProfileId(currentProfileId);
  }, [currentProfileId, isDirty, syncedWorkspaceId, workspaceId]);

  useSettingsSaveContributor({
    id: "utility-config-chat-agent",
    order: 20,
    revision: draftProfileId,
    isDirty: Boolean(workspace) && isDirty,
    save: async () => {
      if (!workspace) return;
      const submitted = draftProfileId;
      await updateWorkspaceAction(workspace.id, {
        default_config_agent_profile_id: submitted,
      });
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(
        workspaces.items.map((w) =>
          w.id === workspace.id ? { ...w, default_config_agent_profile_id: submitted } : w,
        ),
      );
      setSavedProfileId(submitted);
    },
    discard: () => setDraftProfileId(savedProfileId),
  });

  if (!workspace) return null;

  return (
    <SettingsCard isDirty={isDirty} data-testid="config-chat-agent-card">
      <SettingsCardHeader title={t("settings:configChatAgentTitle")} />
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{t("settings:configChatAgentDescription")}</p>
        <div className="space-y-2" data-settings-dirty={isDirty}>
          <SettingsFieldLabel>{t("settings:utilityAgentProfile")}</SettingsFieldLabel>
          <UtilityAgentProfilePicker
            profiles={profiles}
            value={draftProfileId || "none"}
            onValueChange={(value) => setDraftProfileId(value === "none" ? "" : value)}
            fallback={{ value: "none", label: t("settings:configChatAgentNoDefault") }}
            unavailableValue={
              draftProfileId && !profiles.some((profile) => profile.id === draftProfileId)
                ? draftProfileId
                : undefined
            }
            testId="utility-profile-picker-config-chat"
            triggerClassName="w-full max-w-sm font-normal"
            includeWorkspaceProfiles
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
