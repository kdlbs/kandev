"use client";

import { useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { SettingsField } from "./settings-field";
import { settingsControlClassName, settingsCredentialClassName } from "./settings-control";
import {
  DEFAULT_KUBERNETES_IMAGE,
  isKubernetesProfileDirty,
  type KubernetesPlatform,
  type KubernetesProfileConfigForm,
} from "./kubernetes-config";

// i18n-exempt: persisted Kubernetes platform identifiers shown verbatim.
const PLATFORM_OPTIONS: KubernetesPlatform[] = ["linux/amd64", "linux/arm64"];

type KubernetesWorkloadCardProps = {
  form: KubernetesProfileConfigForm;
  baseline: KubernetesProfileConfigForm;
  onChange: (form: KubernetesProfileConfigForm) => void;
  readOnly?: boolean;
};

export function KubernetesWorkloadCard({
  form,
  baseline,
  onChange,
  readOnly = false,
}: KubernetesWorkloadCardProps) {
  const { t } = useTranslation();
  const podTemplateRef = useRef<HTMLTextAreaElement>(null);
  const update = <K extends keyof KubernetesProfileConfigForm>(
    key: K,
    value: KubernetesProfileConfigForm[K],
  ) => onChange({ ...form, [key]: value });

  useLayoutEffect(() => {
    const textarea = podTemplateRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [form.podTemplateYaml]);

  return (
    <SettingsCard
      className="min-w-0 overflow-hidden"
      isDirty={isKubernetesProfileDirty(form, baseline)}
      data-testid="kubernetes-workload-card"
    >
      <SettingsCardHeader
        title={t("executors:kubernetesWorkloadTitle")}
        description={t("executors:kubernetesWorkloadDescription")}
      />
      <CardContent>
        <fieldset disabled={readOnly} className="grid min-w-0 gap-5 md:grid-cols-2">
          <SettingsField
            label={t("executors:kubernetesPlatform")}
            helper={t("executors:kubernetesPlatformHelp")}
            labelProps={{ htmlFor: "kubernetes-platform" }}
          >
            <Select
              value={form.platform}
              onValueChange={(value) =>
                update("platform", value as KubernetesProfileConfigForm["platform"])
              }
            >
              <SelectTrigger
                id="kubernetes-platform"
                data-testid="kubernetes-platform"
                className={settingsControlClassName("w-full")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PLATFORM_OPTIONS.map((platform) => (
                  <SelectItem key={platform} value={platform}>
                    {platform}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsField>
          <SettingsField
            label={t("executors:kubernetesMainContainer")}
            helper={t("executors:kubernetesMainContainerHelp")}
            labelProps={{ htmlFor: "kubernetes-main-container" }}
          >
            <Input
              id="kubernetes-main-container"
              data-testid="kubernetes-main-container"
              value={form.mainContainer}
              onChange={(event) => update("mainContainer", event.target.value)}
              className={settingsCredentialClassName()}
            />
          </SettingsField>
          <SettingsField
            className="min-w-0 md:col-span-2"
            label={t("executors:kubernetesPodTemplate")}
            helper={t("executors:kubernetesPodTemplateHelp", {
              image: DEFAULT_KUBERNETES_IMAGE,
            })}
            labelProps={{ htmlFor: "kubernetes-pod-template" }}
          >
            <Textarea
              ref={podTemplateRef}
              id="kubernetes-pod-template"
              data-testid="kubernetes-pod-template"
              value={form.podTemplateYaml}
              onChange={(event) => update("podTemplateYaml", event.target.value)}
              wrap="off"
              spellCheck={false}
              className="field-sizing-fixed min-h-28 w-full max-w-full resize-none overflow-x-auto overflow-y-hidden whitespace-pre font-mono text-base md:text-xs"
            />
          </SettingsField>
          <p className="text-xs text-amber-700 dark:text-amber-300 md:col-span-2">
            {t("executors:kubernetesPodTemplateWarning")}
          </p>
        </fieldset>
      </CardContent>
    </SettingsCard>
  );
}
