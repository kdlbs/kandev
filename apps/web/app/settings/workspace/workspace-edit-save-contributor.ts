import { useTranslation } from "react-i18next";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";

// The workspace overview form's registration with the settings save bar.

export type WorkspaceFormSaveOptions = {
  workspaceId: string;
  workspaceNameDraft: string;
  defaultExecutorId: string;
  defaultAgentProfileId: string;
  idleSuspensionEnabled: boolean;
  idleTimeoutMinutes: string;
  midTurnFailoverEnabled: boolean;
  failedRouteAutoRetryEnabled: boolean;
  failedRouteAutoRetryMax: string;
  idleTimeoutValid: boolean;
  autoRetryMaxValid: boolean;
  isDirty: boolean;
  save: () => Promise<void>;
  discard: () => void;
};

export function useWorkspaceFormSaveContributor(options: WorkspaceFormSaveOptions) {
  const { t } = useTranslation();
  const hasWorkspaceName = Boolean(options.workspaceNameDraft.trim());
  const invalidReason = workspaceDraftInvalidReason(
    hasWorkspaceName,
    options.idleTimeoutValid,
    options.autoRetryMaxValid,
    t,
  );
  useSettingsSaveContributor({
    id: `workspace:${options.workspaceId}`,
    revision: JSON.stringify({
      workspaceNameDraft: options.workspaceNameDraft,
      defaultExecutorId: options.defaultExecutorId,
      defaultAgentProfileId: options.defaultAgentProfileId,
      idleSuspensionEnabled: options.idleSuspensionEnabled,
      idleTimeoutMinutes: options.idleTimeoutMinutes,
      midTurnFailoverEnabled: options.midTurnFailoverEnabled,
      failedRouteAutoRetryEnabled: options.failedRouteAutoRetryEnabled,
      failedRouteAutoRetryMax: options.failedRouteAutoRetryMax,
    }),
    isDirty: options.isDirty,
    canSave: invalidReason === undefined,
    invalidReason,
    save: options.save,
    discard: options.discard,
  });
}

function workspaceDraftInvalidReason(
  hasWorkspaceName: boolean,
  timeoutValid: boolean,
  autoRetryMaxValid: boolean,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!hasWorkspaceName) return t("workspaces:workspaceNameIsRequired");
  if (!timeoutValid) return t("workspaces:idleTimeoutMustBePositive");
  if (!autoRetryMaxValid) return t("workspaces:failedRouteAutoRetryMaxRange");
  return undefined;
}
