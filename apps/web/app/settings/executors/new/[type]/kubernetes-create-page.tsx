"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { KubernetesConnectionCard } from "@/components/settings/kubernetes-connection-card";
import { kubernetesCreateErrorMessage } from "@/components/settings/kubernetes-create-error";
import { KubernetesDiagnosticsCard } from "@/components/settings/kubernetes-diagnostics-card";
import { KubernetesReadOnlyNotice } from "@/components/settings/kubernetes-read-only-notice";
import { KubernetesWorkloadCard } from "@/components/settings/kubernetes-workload-card";
import { KubernetesWorkspaceCard } from "@/components/settings/kubernetes-workspace-card";
import { ProfileDetailsCard } from "@/components/settings/profile-edit/profile-details-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { serializeSettingsRevision } from "@/components/settings/settings-save-revision";
import {
  createDefaultKubernetesExecutorForm,
  createDefaultKubernetesProfileConfig,
  serializeKubernetesExecutorConfig,
  serializeKubernetesProfileConfig,
  type KubernetesExecutorForm,
  type KubernetesProfileConfigForm,
} from "@/components/settings/kubernetes-config";
import {
  getKubernetesCreateContributorState,
  kubernetesProfileInvalidReason,
} from "@/components/settings/kubernetes-validation";
import {
  useKubernetesAdminAccess,
  useKubernetesDiagnostics,
  useKubernetesExecutorResource,
} from "@/hooks/domains/settings/use-kubernetes-settings";
import { getExecutorIcon, getExecutorLabel } from "@/lib/executor-icons";
import { executorProfileSettingsPath } from "@/lib/settings/executor-settings-routes";

const EXECUTORS_ROUTE = "/settings/executors";
const KubernetesIcon = getExecutorIcon("k8s");

export function KubernetesCreatePage() {
  const { t } = useTranslation();
  const router = useRouter();
  const canManage = useKubernetesAdminAccess();
  const resource = useKubernetesExecutorResource();
  const diagnostics = useKubernetesDiagnostics();
  const [executorBaseline] = useState(createDefaultKubernetesExecutorForm);
  const [profileBaseline] = useState(createDefaultKubernetesProfileConfig);
  const [executor, setExecutor] = useState<KubernetesExecutorForm>(executorBaseline);
  const [profileName, setProfileName] = useState("");
  const [profile, setProfile] = useState<KubernetesProfileConfigForm>(profileBaseline);
  const [error, setError] = useState<unknown>(null);

  const payload = useMemo(
    () => ({
      name: executor.name.trim(),
      config: serializeKubernetesExecutorConfig(executor),
      profileName: profileName.trim(),
      profileConfig: serializeKubernetesProfileConfig(profile),
    }),
    [executor, profile, profileName],
  );
  const invalidReason = createInvalidReason(executor, profileName, profile, canManage, t);
  const contributorState = getKubernetesCreateContributorState(canManage, invalidReason);
  const revision = serializeSettingsRevision(payload);

  const save = useCallback(async () => {
    setError(null);
    try {
      const created = await resource.create(payload);
      runWithNavigationBlockerBypassed(() =>
        router.push(executorProfileSettingsPath(created.executor, created.profile.id)),
      );
    } catch (cause) {
      setError(cause);
      throw cause;
    }
  }, [payload, resource, router]);

  const discard = useCallback(() => {
    setExecutor({ ...executorBaseline });
    setProfileName("");
    setProfile({ ...profileBaseline, accessModes: [...profileBaseline.accessModes] });
    diagnostics.clear();
    setError(null);
  }, [diagnostics, executorBaseline, profileBaseline]);

  useSettingsSaveContributor({
    id: "kubernetes-executor:create",
    revision,
    isDirty: contributorState.isDirty,
    canSave: contributorState.canSave,
    invalidReason,
    save,
    discard,
  });

  const runTest = () => {
    void diagnostics
      .run({ config: payload.config, profile_config: payload.profileConfig })
      .catch(() => undefined);
  };
  const errorMessage = kubernetesCreateErrorMessage(error, t);

  return (
    <div className="min-w-0 space-y-8 overflow-x-clip">
      <KubernetesCreateHeader />
      {!canManage && <KubernetesReadOnlyNotice />}
      <KubernetesConnectionCard
        form={executor}
        baseline={executorBaseline}
        onChange={setExecutor}
        readOnly={!canManage}
      />
      <fieldset disabled={!canManage} className="min-w-0 space-y-8">
        <ProfileDetailsCard name={profileName} baselineName="" onNameChange={setProfileName} />
        <KubernetesWorkloadCard form={profile} baseline={profileBaseline} onChange={setProfile} />
        <KubernetesWorkspaceCard form={profile} baseline={profileBaseline} onChange={setProfile} />
      </fieldset>
      <KubernetesDiagnosticsCard
        testing={diagnostics.testing}
        result={diagnostics.result}
        error={diagnostics.error}
        onTest={runTest}
        canManage={canManage}
        includesProfile
      />
      {Boolean(error) && (
        <p role="alert" className="break-words text-sm text-destructive">
          {errorMessage}
        </p>
      )}
    </div>
  );
}

function KubernetesCreateHeader() {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <>
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <KubernetesIcon className="h-5 w-5 text-muted-foreground" />
            <h2 className="min-w-0 break-words text-2xl font-bold">
              {t("executors:kubernetesCreateTitle")}
            </h2>
            <Badge variant="outline" className="text-[10px]">
              {getExecutorLabel("k8s")}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("executors:kubernetesCreateDescription")}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="min-h-11 w-full cursor-pointer text-sm md:min-h-7 md:w-auto md:text-xs"
        >
          {t("executors:backToExecutors")}
        </Button>
      </div>
      <Separator />
    </>
  );
}

function createInvalidReason(
  executor: KubernetesExecutorForm,
  profileName: string,
  profile: KubernetesProfileConfigForm,
  canManage: boolean,
  t: (key: string) => string,
): string | undefined {
  if (!canManage) return t("executors:kubernetesAdminSaveOnly");
  if (!executor.name.trim()) return t("executors:kubernetesExecutorNameRequired");
  if (!executor.namespace.trim()) return t("executors:kubernetesNamespaceRequired");
  if (executor.authMode === "kubeconfig" && !executor.kubeconfigPath.trim()) {
    return t("executors:kubernetesKubeconfigPathRequired");
  }
  const timeout = Number(executor.requestTimeoutSeconds);
  if (!Number.isInteger(timeout) || timeout < 1 || timeout > 300) {
    return t("executors:kubernetesTimeoutInvalid");
  }
  if (!profileName.trim()) return t("executors:profileNameIsRequired");
  return kubernetesProfileInvalidReason(profile, t);
}
