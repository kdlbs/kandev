"use client";

import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { SettingsField } from "./settings-field";
import { settingsControlClassName, settingsCredentialClassName } from "./settings-control";
import { isKubernetesExecutorDirty, type KubernetesExecutorForm } from "./kubernetes-config";

// i18n-exempt: example persisted Kubernetes namespace identifier.
const KUBERNETES_NAMESPACE_EXAMPLE = "agents";
// i18n-exempt: example absolute filesystem path, rendered verbatim.
const KUBECONFIG_PATH_EXAMPLE = "/etc/kandev/kubeconfig";

type KubernetesConnectionCardProps = {
  form: KubernetesExecutorForm;
  baseline: KubernetesExecutorForm;
  onChange: (form: KubernetesExecutorForm) => void;
  readOnly?: boolean;
};

export function KubernetesConnectionCard({
  form,
  baseline,
  onChange,
  readOnly = false,
}: KubernetesConnectionCardProps) {
  const { t } = useTranslation();
  const update = <K extends keyof KubernetesExecutorForm>(
    key: K,
    value: KubernetesExecutorForm[K],
  ) => onChange({ ...form, [key]: value });

  return (
    <SettingsCard
      className="min-w-0 overflow-hidden"
      isDirty={isKubernetesExecutorDirty(form, baseline)}
      data-testid="kubernetes-connection-card"
    >
      <SettingsCardHeader
        title={t("executors:kubernetesConnectionTitle")}
        description={t("executors:kubernetesConnectionDescription")}
      />
      <CardContent>
        <fieldset disabled={readOnly} className="grid min-w-0 gap-5 md:grid-cols-2">
          <SettingsField
            className="md:col-span-2"
            label={t("executors:kubernetesExecutorName")}
            helper={t("executors:kubernetesExecutorNameHelp")}
            labelProps={{ htmlFor: "kubernetes-executor-name" }}
          >
            <Input
              id="kubernetes-executor-name"
              data-testid="kubernetes-executor-name"
              value={form.name}
              onChange={(event) => update("name", event.target.value)}
              className={settingsControlClassName()}
            />
          </SettingsField>
          <KubernetesAuthenticationField form={form} update={update} />
          <SettingsField
            label={t("executors:kubernetesNamespace")}
            helper={t("executors:kubernetesNamespaceHelp")}
            labelProps={{ htmlFor: "kubernetes-namespace" }}
          >
            <Input
              id="kubernetes-namespace"
              data-testid="kubernetes-namespace"
              value={form.namespace}
              onChange={(event) => update("namespace", event.target.value)}
              placeholder={KUBERNETES_NAMESPACE_EXAMPLE}
              className={settingsCredentialClassName()}
            />
          </SettingsField>
          {form.authMode === "kubeconfig" && <KubeconfigFields form={form} update={update} />}
          <SettingsField
            label={t("executors:kubernetesRequestTimeout")}
            helper={t("executors:kubernetesRequestTimeoutHelp")}
            labelProps={{ htmlFor: "kubernetes-request-timeout" }}
          >
            <Input
              id="kubernetes-request-timeout"
              data-testid="kubernetes-request-timeout"
              type="number"
              min={1}
              max={300}
              inputMode="numeric"
              value={form.requestTimeoutSeconds}
              onChange={(event) => update("requestTimeoutSeconds", event.target.value)}
              className={settingsControlClassName()}
            />
          </SettingsField>
        </fieldset>
      </CardContent>
    </SettingsCard>
  );
}

type UpdateExecutorField = <K extends keyof KubernetesExecutorForm>(
  key: K,
  value: KubernetesExecutorForm[K],
) => void;

function KubernetesAuthenticationField({
  form,
  update,
}: {
  form: KubernetesExecutorForm;
  update: UpdateExecutorField;
}) {
  const { t } = useTranslation();
  const helperKey =
    form.authMode === "kubeconfig"
      ? "executors:kubernetesKubeconfigAuthHelp"
      : "executors:kubernetesInClusterAuthHelp";
  return (
    <SettingsField
      label={t("executors:kubernetesAuthMode")}
      helper={t(helperKey)}
      labelProps={{ htmlFor: "kubernetes-auth-mode" }}
    >
      <Select
        value={form.authMode}
        onValueChange={(value) => update("authMode", value as KubernetesExecutorForm["authMode"])}
      >
        <SelectTrigger
          id="kubernetes-auth-mode"
          data-testid="kubernetes-auth-mode"
          className={settingsControlClassName("w-full")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="kubeconfig">{t("executors:kubernetesAuthKubeconfig")}</SelectItem>
          <SelectItem value="in_cluster">{t("executors:kubernetesAuthInCluster")}</SelectItem>
        </SelectContent>
      </Select>
    </SettingsField>
  );
}

function KubeconfigFields({
  form,
  update,
}: {
  form: KubernetesExecutorForm;
  update: UpdateExecutorField;
}) {
  const { t } = useTranslation();
  return (
    <>
      <SettingsField
        label={t("executors:kubernetesKubeconfigPath")}
        helper={t("executors:kubernetesKubeconfigPathHelp")}
        labelProps={{ htmlFor: "kubernetes-kubeconfig-path" }}
      >
        <Input
          id="kubernetes-kubeconfig-path"
          data-testid="kubernetes-kubeconfig-path"
          value={form.kubeconfigPath}
          onChange={(event) => update("kubeconfigPath", event.target.value)}
          placeholder={KUBECONFIG_PATH_EXAMPLE}
          className={settingsCredentialClassName()}
        />
      </SettingsField>
      <SettingsField
        label={t("executors:kubernetesContext")}
        helper={t("executors:kubernetesContextHelp")}
        labelProps={{ htmlFor: "kubernetes-context" }}
      >
        <Input
          id="kubernetes-context"
          data-testid="kubernetes-context"
          value={form.kubeContext}
          onChange={(event) => update("kubeContext", event.target.value)}
          className={settingsCredentialClassName()}
        />
      </SettingsField>
    </>
  );
}
