"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertCircle, IconLock } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import {
  fetchMessageQueueSettings,
  updateMessageQueueSettings,
} from "@/lib/api/domains/settings-api";
import type { MessageQueueSettingsResponse, MessageQueueSettingsSource } from "@/lib/types/system";

const ENVIRONMENT_VARIABLE = "KANDEV_QUEUE_MAX_PER_SESSION";

function parseMaximum(value: string): number | null {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

function sourceLabelKey(source: MessageQueueSettingsSource): string {
  switch (source) {
    case "setting":
      return "system:messageQueueSourceSetting";
    case "environment":
      return "system:messageQueueSourceEnvironment";
    default:
      return "system:messageQueueSourceDefault";
  }
}

function useMessageQueueSettingsDraft() {
  const { t } = useTranslation();
  const role = useAppStore((state) => state.auth.user?.role);
  const [snapshot, setSnapshot] = useState<MessageQueueSettingsResponse | null>(null);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [saveFailed, setSaveFailed] = useState(false);
  const loadVersion = useRef(0);

  const reload = useCallback(async () => {
    const version = ++loadVersion.current;
    setLoading(true);
    setLoadFailed(false);
    try {
      const response = await fetchMessageQueueSettings();
      if (version !== loadVersion.current) return;
      setSnapshot(response);
      setDraft(String(response.settings.max_per_session));
    } catch {
      if (version === loadVersion.current) setLoadFailed(true);
    } finally {
      if (version === loadVersion.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    return () => {
      loadVersion.current += 1;
    };
  }, [reload]);

  const parsed = parseMaximum(draft);
  const baseline = snapshot?.settings.max_per_session;
  const isDirty = baseline !== undefined && draft !== String(baseline);
  const isAdmin = role === undefined || role === "admin";
  const isLocked = snapshot?.effective.locked === true;
  const canEdit = isAdmin && !isLocked;
  let invalidReason: string | undefined;
  if (parsed === null) invalidReason = t("system:messageQueueValidation");
  else if (!isAdmin) invalidReason = t("system:messageQueueAdminOnly");
  else if (isLocked) {
    invalidReason = t("system:messageQueueEnvironmentLocked", {
      variable: ENVIRONMENT_VARIABLE,
    });
  }

  useSettingsSaveContributor({
    id: "system-message-queue",
    revision: draft,
    isDirty,
    canSave: parsed !== null && canEdit,
    invalidReason,
    save: async () => {
      if (parsed === null || !canEdit) throw new Error(invalidReason);
      const submitted = draft;
      setSaveFailed(false);
      try {
        const response = await updateMessageQueueSettings({ max_per_session: parsed });
        setSnapshot(response);
        setDraft((current) =>
          current === submitted ? String(response.settings.max_per_session) : current,
        );
      } catch (error) {
        setSaveFailed(true);
        throw error;
      }
    },
    discard: () => {
      if (baseline !== undefined) setDraft(String(baseline));
      setSaveFailed(false);
    },
  });

  return {
    snapshot,
    draft,
    setDraft,
    loading,
    loadFailed,
    saveFailed,
    isDirty,
    isAdmin,
    isLocked,
    reload,
  };
}

export function MessageQueueSettings() {
  const { t } = useTranslation();
  const state = useMessageQueueSettingsDraft();

  if (state.loading && !state.snapshot) {
    return (
      <SettingsCard>
        <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <Spinner className="size-4" />
          {t("system:messageQueueLoading")}
        </CardContent>
      </SettingsCard>
    );
  }

  if (state.loadFailed && !state.snapshot) {
    return <MessageQueueLoadError onRetry={() => void state.reload()} />;
  }

  if (!state.snapshot) return null;
  const { settings, effective } = state.snapshot;
  const effectiveValue =
    effective.max_per_session === 0
      ? t("system:messageQueueUnlimited")
      : String(effective.max_per_session);

  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full max-w-3xl"
      data-testid="message-queue-settings"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("system:messageQueueLimitTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="min-w-0 space-y-5">
        <p className="text-sm text-muted-foreground">{t("system:messageQueueLimitDescription")}</p>
        <div className="min-w-0 space-y-2">
          <Label htmlFor="message-queue-max-per-session">
            {t("system:messageQueueMaximumLabel")}
          </Label>
          <Input
            id="message-queue-max-per-session"
            data-testid="message-queue-max-per-session"
            type="number"
            inputMode="numeric"
            min={0}
            step={1}
            value={state.draft}
            disabled={!state.isAdmin || state.isLocked}
            onChange={(event) => state.setDraft(event.target.value)}
            className="h-11 w-full max-w-xs"
          />
          <p className="text-xs text-muted-foreground">{t("system:messageQueueUnlimitedHelp")}</p>
        </div>

        <div className="rounded-md border border-border/70 bg-muted/20 p-3 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-muted-foreground">{t("system:messageQueueConfigured")}</span>
            <span>{settings.max_per_session}</span>
            <span className="text-muted-foreground">{t("system:messageQueueEffective")}</span>
            <strong data-testid="message-queue-effective-value">{effectiveValue}</strong>
            <Badge variant="secondary" data-testid="message-queue-source">
              {t(sourceLabelKey(effective.source))}
            </Badge>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {t("system:messageQueueEffectiveHelp")}
          </p>
        </div>

        {state.isLocked && (
          <Alert>
            <IconLock className="size-4" />
            <AlertTitle>{t("system:messageQueueEnvironmentLockTitle")}</AlertTitle>
            <AlertDescription>
              {t("system:messageQueueEnvironmentLocked", { variable: ENVIRONMENT_VARIABLE })}
            </AlertDescription>
          </Alert>
        )}
        {!state.isAdmin && (
          <p className="text-sm text-muted-foreground">{t("system:messageQueueAdminOnly")}</p>
        )}
        {state.saveFailed && (
          <Alert variant="destructive">
            <IconAlertCircle className="size-4" />
            <AlertDescription>{t("system:messageQueueSaveFailed")}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </SettingsCard>
  );
}

function MessageQueueLoadError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <SettingsCard>
      <CardContent className="space-y-3 py-6">
        <Alert variant="destructive">
          <IconAlertCircle className="size-4" />
          <AlertDescription>{t("system:messageQueueLoadFailed")}</AlertDescription>
        </Alert>
        <Button variant="outline" className="h-11 cursor-pointer" onClick={onRetry}>
          {t("system:messageQueueRetry")}
        </Button>
      </CardContent>
    </SettingsCard>
  );
}
