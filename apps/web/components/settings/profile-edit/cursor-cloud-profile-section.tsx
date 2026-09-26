"use client";

import { useCallback, useState } from "react";
import { CursorCloudConfigCard } from "@/components/settings/profile-edit/cursor-cloud-config-card";
import { buildCursorCloudProfileConfig } from "@/components/settings/profile-edit/cursor-cloud-profile-config";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { ExecutorProfile } from "@/lib/types/http";

export function useCursorCloudProfileSettings(profile: ExecutorProfile) {
  const [secretId, setSecretId] = useState<string | null>(
    () => profile.config?.cursor_cloud_api_key_secret_id ?? null,
  );
  const [callbackUrl, setCallbackUrl] = useState(
    () => profile.config?.cursor_cloud_callback_url ?? "",
  );

  const reset = useCallback(() => {
    setSecretId(profile.config?.cursor_cloud_api_key_secret_id ?? null);
    setCallbackUrl(profile.config?.cursor_cloud_callback_url ?? "");
  }, [profile]);
  const applyToConfig = useCallback(
    (config: Record<string, string>) =>
      buildCursorCloudProfileConfig(config, secretId, callbackUrl),
    [callbackUrl, secretId],
  );

  return { secretId, setSecretId, callbackUrl, setCallbackUrl, reset, applyToConfig };
}

export type CursorCloudProfileSettings = ReturnType<typeof useCursorCloudProfileSettings>;

export function CursorCloudProfileSection({
  profile,
  secrets,
  settings,
}: {
  profile: ExecutorProfile;
  secrets: SecretListItem[];
  settings: CursorCloudProfileSettings;
}) {
  return (
    <CursorCloudConfigCard
      secretId={settings.secretId}
      baselineSecretId={profile.config?.cursor_cloud_api_key_secret_id ?? null}
      callbackUrl={settings.callbackUrl}
      baselineCallbackUrl={profile.config?.cursor_cloud_callback_url ?? ""}
      secrets={secrets}
      onSecretIdChange={settings.setSecretId}
      onCallbackUrlChange={settings.setCallbackUrl}
    />
  );
}
