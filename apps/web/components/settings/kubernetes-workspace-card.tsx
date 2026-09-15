"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { SettingsField, SettingsFieldDescription, SettingsFieldLabel } from "./settings-field";
import { settingsControlClassName, settingsCredentialClassName } from "./settings-control";
import {
  isKubernetesProfileDirty,
  type KubernetesAccessMode,
  type KubernetesProfileConfigForm,
} from "./kubernetes-config";

// i18n-exempt: persisted Kubernetes access-mode identifiers shown verbatim.
const ACCESS_MODES: KubernetesAccessMode[] = [
  "ReadWriteOnce",
  "ReadOnlyMany",
  "ReadWriteMany",
  "ReadWriteOncePod",
];

// i18n-exempt: example Kubernetes resource quantity rendered verbatim.
const WORKSPACE_SIZE_EXAMPLE = "10Gi";

type KubernetesWorkspaceCardProps = {
  form: KubernetesProfileConfigForm;
  baseline: KubernetesProfileConfigForm;
  onChange: (form: KubernetesProfileConfigForm) => void;
  readOnly?: boolean;
};

export function KubernetesWorkspaceCard({
  form,
  baseline,
  onChange,
  readOnly = false,
}: KubernetesWorkspaceCardProps) {
  const { t } = useTranslation();
  const update = <K extends keyof KubernetesProfileConfigForm>(
    key: K,
    value: KubernetesProfileConfigForm[K],
  ) => onChange({ ...form, [key]: value });

  return (
    <SettingsCard
      className="min-w-0 overflow-hidden"
      isDirty={isKubernetesProfileDirty(form, baseline)}
      data-testid="kubernetes-workspace-card"
    >
      <SettingsCardHeader
        title={t("executors:kubernetesWorkspaceTitle")}
        description={t("executors:kubernetesWorkspaceDescription")}
      />
      <CardContent>
        <fieldset disabled={readOnly} className="space-y-5">
          <SettingsField
            label={t("executors:kubernetesWorkspaceMode")}
            helper={t("executors:kubernetesWorkspaceModeHelp")}
            labelProps={{ htmlFor: "kubernetes-workspace-mode" }}
          >
            <Select
              value={form.workspaceMode}
              onValueChange={(value) =>
                update("workspaceMode", value as KubernetesProfileConfigForm["workspaceMode"])
              }
            >
              <SelectTrigger
                id="kubernetes-workspace-mode"
                data-testid="kubernetes-workspace-mode"
                className={settingsControlClassName("w-full")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="managed_pvc">
                  {t("executors:kubernetesWorkspaceManagedPvc")}
                </SelectItem>
                <SelectItem value="empty_dir">
                  {t("executors:kubernetesWorkspaceEmptyDir")}
                </SelectItem>
                <SelectItem value="existing_claim">
                  {t("executors:kubernetesWorkspaceExistingClaim")}
                </SelectItem>
              </SelectContent>
            </Select>
          </SettingsField>
          {form.workspaceMode === "managed_pvc" && <ManagedPvcFields form={form} update={update} />}
          {form.workspaceMode === "empty_dir" && (
            <SettingsFieldDescription data-testid="kubernetes-empty-dir-warning">
              {t("executors:kubernetesEmptyDirWarning")}
            </SettingsFieldDescription>
          )}
          {form.workspaceMode === "existing_claim" && (
            <ExistingClaimField form={form} update={update} />
          )}
        </fieldset>
      </CardContent>
    </SettingsCard>
  );
}

type UpdateProfileField = <K extends keyof KubernetesProfileConfigForm>(
  key: K,
  value: KubernetesProfileConfigForm[K],
) => void;

function ManagedPvcFields({
  form,
  update,
}: {
  form: KubernetesProfileConfigForm;
  update: UpdateProfileField;
}) {
  const { t } = useTranslation();
  const toggleAccessMode = (mode: KubernetesAccessMode, checked: boolean) => {
    const next = checked
      ? [...new Set([...form.accessModes, mode])]
      : form.accessModes.filter((value) => value !== mode);
    update("accessModes", next);
  };
  return (
    <div className="grid gap-5 md:grid-cols-2">
      <SettingsField
        label={t("executors:kubernetesWorkspaceSize")}
        helper={t("executors:kubernetesWorkspaceSizeHelp")}
        labelProps={{ htmlFor: "kubernetes-workspace-size" }}
      >
        <Input
          id="kubernetes-workspace-size"
          data-testid="kubernetes-workspace-size"
          value={form.workspaceSize}
          onChange={(event) => update("workspaceSize", event.target.value)}
          placeholder={WORKSPACE_SIZE_EXAMPLE}
          className={settingsCredentialClassName()}
        />
      </SettingsField>
      <SettingsField
        label={t("executors:kubernetesStorageClass")}
        helper={t("executors:kubernetesStorageClassHelp")}
        labelProps={{ htmlFor: "kubernetes-storage-class" }}
      >
        <Input
          id="kubernetes-storage-class"
          data-testid="kubernetes-storage-class"
          value={form.storageClass}
          onChange={(event) => update("storageClass", event.target.value)}
          className={settingsCredentialClassName()}
        />
      </SettingsField>
      <div className="space-y-2 md:col-span-2">
        <SettingsFieldLabel>{t("executors:kubernetesAccessModes")}</SettingsFieldLabel>
        <SettingsFieldDescription>
          {t("executors:kubernetesAccessModesHelp")}
        </SettingsFieldDescription>
        <div className="grid gap-2 sm:grid-cols-2">
          {ACCESS_MODES.map((mode) => (
            <label key={mode} className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
              <Checkbox
                checked={form.accessModes.includes(mode)}
                onCheckedChange={(checked) => toggleAccessMode(mode, checked === true)}
              />
              <span className="font-mono text-xs">{mode}</span>
            </label>
          ))}
        </div>
      </div>
    </div>
  );
}

function ExistingClaimField({
  form,
  update,
}: {
  form: KubernetesProfileConfigForm;
  update: UpdateProfileField;
}) {
  const { t } = useTranslation();
  return (
    <SettingsField
      label={t("executors:kubernetesClaimName")}
      helper={t("executors:kubernetesClaimNameHelp")}
      labelProps={{ htmlFor: "kubernetes-claim-name" }}
    >
      <Input
        id="kubernetes-claim-name"
        data-testid="kubernetes-claim-name"
        value={form.claimName}
        onChange={(event) => update("claimName", event.target.value)}
        className={settingsCredentialClassName()}
      />
    </SettingsField>
  );
}
