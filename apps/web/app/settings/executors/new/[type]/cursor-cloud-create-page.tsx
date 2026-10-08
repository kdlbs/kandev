"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { useAppStoreApi } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { executorProfileSettingsPath } from "@/lib/settings/executor-settings-routes";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { serializeSettingsRevision } from "@/components/settings/settings-save-revision";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { ProfileDetailsCard } from "@/components/settings/profile-edit/profile-details-card";
import { CursorCloudConfigCard } from "@/components/settings/profile-edit/cursor-cloud-config-card";
import { buildCursorCloudProfileConfig } from "@/components/settings/profile-edit/cursor-cloud-profile-config";
import { createCursorCloudProfile } from "./create-cursor-cloud-profile";

export function CursorCloudCreatePage() {
  const { t } = useTranslation();
  const router = useRouter();
  const store = useAppStoreApi();
  const { items: secrets } = useSecrets();
  const [name, setName] = useState("");
  const [secretId, setSecretId] = useState<string | null>(null);
  const [callbackUrl, setCallbackUrl] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const payload = {
    name: name.trim(),
    config: buildCursorCloudProfileConfig({}, secretId, callbackUrl),
    prepare_script: "",
    cleanup_script: "",
  };
  const save = useCallback(async () => {
    setSaving(true);
    setError(null);
    try {
      const executor = await createCursorCloudProfile(payload);
      const current = store.getState().executors.items;
      store
        .getState()
        .setExecutors([...current.filter((item) => item.id !== executor.id), executor]);
      runWithNavigationBlockerBypassed(() =>
        router.push(executorProfileSettingsPath(executor.profiles![0].id)),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("executors:failedToCreateProfile"));
      throw cause;
    } finally {
      setSaving(false);
    }
  }, [payload, router, store, t]);
  useSettingsSaveContributor({
    id: "executor-profile:new:cursor_cloud",
    revision: serializeSettingsRevision(payload),
    isDirty: Boolean(name || secretId || callbackUrl),
    canSave: Boolean(name.trim() && secretId && callbackUrl.trim()) && !saving,
    save,
    discard: () => {
      setName("");
      setSecretId(null);
      setCallbackUrl("");
    },
  });

  return (
    <div className="space-y-8">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <h2 className="text-2xl font-bold">
            {t("executors:newTypeProfile", { type: t("executors:cursorCloudTitle") })}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("executors:cursorCloudDescription")}
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => router.push("/settings/executors")}
          className={settingsActionClassName("w-full cursor-pointer md:w-auto")}
        >
          {t("executors:backToExecutors")}
        </Button>
      </div>
      <Separator />
      <fieldset disabled={saving} className="space-y-8">
        <ProfileDetailsCard name={name} baselineName="" onNameChange={setName} />
        <CursorCloudConfigCard
          secretId={secretId}
          baselineSecretId={null}
          callbackUrl={callbackUrl}
          baselineCallbackUrl=""
          secrets={secrets}
          onSecretIdChange={setSecretId}
          onCallbackUrlChange={setCallbackUrl}
        />
      </fieldset>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
