"use client";

import { useCallback, useState } from "react";
import { IconCheck, IconLoader2, IconTestPipe, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsField } from "@/components/settings/settings-field";
import { settingsControlClassName } from "@/components/settings/settings-control";
import { loadCursorCloudCatalog, type CursorCloudModel } from "@/lib/api/domains/cursor-cloud-api";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { useTranslation } from "react-i18next";

type CursorCloudConfigCardProps = {
  secretId: string | null;
  baselineSecretId: string | null;
  callbackUrl: string;
  baselineCallbackUrl: string;
  secrets: SecretListItem[];
  onSecretIdChange: (secretId: string | null) => void;
  onCallbackUrlChange: (callbackUrl: string) => void;
};

function CursorCloudConnectionStatus({
  connected,
  error,
}: {
  connected: boolean;
  error: string | null;
}) {
  const { t } = useTranslation();
  if (!connected && !error) return null;
  if (connected) {
    return (
      <Badge variant="default" className="bg-green-600">
        <IconCheck className="mr-1 size-3.5" />
        {t("executors:connected")}
      </Badge>
    );
  }
  return (
    <Badge variant="destructive">
      <IconX className="mr-1 size-3.5" />
      {t("executors:disconnected")}
    </Badge>
  );
}

function CursorCloudModelList({ models }: { models: CursorCloudModel[] }) {
  const { t } = useTranslation();
  if (models.length === 0) return null;
  return (
    <div>
      <p className="font-medium">{t("executors:cursorCloudAvailableModels")}</p>
      <ul className="list-inside list-disc text-muted-foreground">
        {models.map((model) => (
          <li key={model.id}>{model.displayName}</li>
        ))}
      </ul>
    </div>
  );
}

export function CursorCloudConfigCard({
  secretId,
  baselineSecretId,
  callbackUrl,
  baselineCallbackUrl,
  secrets,
  onSecretIdChange,
  onCallbackUrlChange,
}: CursorCloudConfigCardProps) {
  const { t } = useTranslation();
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [models, setModels] = useState<CursorCloudModel[]>([]);
  const dirty = secretId !== baselineSecretId || callbackUrl !== baselineCallbackUrl;

  const testConnection = useCallback(async () => {
    if (!secretId || !callbackUrl.trim()) return;
    setTesting(true);
    setError(null);
    setConnected(false);
    try {
      const result = await loadCursorCloudCatalog(secretId, callbackUrl.trim());
      setModels(result.models ?? []);
      setConnected(result.connected);
    } catch {
      setError(t("executors:cursorCloudConnectionFailed"));
      setModels([]);
    } finally {
      setTesting(false);
    }
  }, [callbackUrl, secretId, t]);

  return (
    <SettingsCard isDirty={dirty}>
      <SettingsCardHeader
        title={t("executors:cursorCloudTitle")}
        description={t("executors:cursorCloudDescription")}
        actions={<CursorCloudConnectionStatus connected={connected} error={error} />}
      />
      <CardContent className="space-y-4">
        <InlineSecretSelect
          secretId={secretId}
          onSecretIdChange={onSecretIdChange}
          secrets={secrets}
          label={t("executors:cursorCloudApiKey")}
          isDirty={secretId !== baselineSecretId}
        />
        <SettingsField label={t("executors:cursorCloudCallbackUrl")}>
          <Input
            type="url"
            value={callbackUrl}
            onChange={(event) => onCallbackUrlChange(event.currentTarget.value)}
            placeholder={t("executors:cursorCloudCallbackPlaceholder")}
            className={settingsControlClassName("text-sm")}
            aria-invalid={Boolean(error)}
          />
        </SettingsField>
        <p className="text-sm text-muted-foreground">{t("executors:cursorCloudBillingNotice")}</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!secretId || !callbackUrl.trim() || testing}
          onClick={() => void testConnection()}
        >
          {testing ? (
            <IconLoader2 className="mr-2 size-4 animate-spin" />
          ) : (
            <IconTestPipe className="mr-2 size-4" />
          )}
          {testing ? t("executors:cursorCloudTesting") : t("executors:cursorCloudTestConnection")}
        </Button>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {connected && (
          <div role="status" className="space-y-2 text-sm">
            <p className="text-green-700 dark:text-green-400">{t("executors:cursorCloudReady")}</p>
            <p className="text-muted-foreground">{t("executors:cursorCloudCallbackNotVerified")}</p>
            <CursorCloudModelList models={models} />
          </div>
        )}
      </CardContent>
    </SettingsCard>
  );
}
