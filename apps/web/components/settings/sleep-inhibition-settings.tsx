"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconAlertCircle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";
import {
  fetchSleepInhibitionSettings,
  updateSleepInhibitionSettings,
} from "@/lib/api/domains/settings-api";
import type { SleepInhibitionResponse } from "@/lib/types/system";

function statusMessageKey(response: SleepInhibitionResponse): string {
  if (response.status.issue === "unsupported_platform")
    return "settings:sleepInhibitionUnsupported";
  if (response.status.issue === "system_service_unavailable") {
    return "settings:sleepInhibitionServiceUnavailable";
  }
  if (response.status.issue === "request_failed") return "settings:sleepInhibitionRequestFailed";
  if (response.status.active) return "settings:sleepInhibitionActive";
  return "settings:sleepInhibitionAvailable";
}

type SleepInhibitionState = {
  snapshot: SleepInhibitionResponse | null;
  draft: boolean;
  setDraft: (value: boolean) => void;
  loading: boolean;
  loadFailed: boolean;
  saveFailed: boolean;
  isDirty: boolean;
  isAdmin: boolean;
  canEdit: boolean;
  saved: boolean | undefined;
  reload: () => Promise<void>;
};

function useSleepInhibitionState(): SleepInhibitionState {
  const { t } = useTranslation();
  const role = useAppStore((state) => state.auth.user?.role);
  const [snapshot, setSnapshot] = useState<SleepInhibitionResponse | null>(null);
  const [draft, setDraft] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [saveFailed, setSaveFailed] = useState(false);
  const loadVersion = useRef(0);
  const isAdmin = role === undefined || role === "admin";
  const saved = snapshot?.settings.enabled;
  const isDirty = saved !== undefined && draft !== saved;
  const canEdit = isAdmin && !loading;

  const reload = useCallback(async () => {
    const version = ++loadVersion.current;
    setLoading(true);
    setLoadFailed(false);
    try {
      const response = await fetchSleepInhibitionSettings();
      if (version !== loadVersion.current) return;
      setSnapshot(response);
      setDraft(response.settings.enabled);
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

  useSettingsSaveContributor({
    id: "general-task-sleep-inhibition",
    order: 30,
    revision: draft ? "enabled" : "disabled",
    isDirty,
    canSave: canEdit,
    invalidReason: !isAdmin ? t("settings:sleepInhibitionAdminOnly") : undefined,
    save: async () => {
      if (!canEdit) throw new Error(t("settings:sleepInhibitionAdminOnly"));
      const submitted = draft;
      setSaveFailed(false);
      try {
        const response = await updateSleepInhibitionSettings({ enabled: submitted });
        setSnapshot(response);
        setDraft((current) => (current === submitted ? response.settings.enabled : current));
      } catch (error) {
        setSaveFailed(true);
        throw error;
      }
    },
    discard: () => {
      if (saved !== undefined) setDraft(saved);
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
    canEdit,
    saved,
    reload,
  };
}

function SleepInhibitionLoadError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <SettingsCard data-testid="sleep-inhibition-settings">
      <CardContent className="py-6">
        <Alert variant="destructive">
          <IconAlertCircle className="size-4" />
          <AlertDescription>{t("settings:sleepInhibitionLoadFailed")}</AlertDescription>
        </Alert>
        <Button variant="outline" className="mt-3 h-11" onClick={onRetry}>
          {t("settings:sleepInhibitionRetry")}
        </Button>
      </CardContent>
    </SettingsCard>
  );
}

function SleepInhibitionCard({ state }: { state: SleepInhibitionState }) {
  const { t } = useTranslation();
  const snapshot = state.snapshot;
  if (!snapshot) return null;
  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full"
      data-testid="sleep-inhibition-settings"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:sleepInhibitionTitle")}</CardTitle>
        <CardDescription>{t("settings:sleepInhibitionDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="min-w-0 space-y-4">
        <div
          className="flex min-h-11 items-center justify-between gap-4"
          data-testid="sleep-inhibition-control-row"
        >
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="task-sleep-inhibition">
              {t("settings:sleepInhibitionSwitchLabel")}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t("settings:sleepInhibitionSwitchHint")}
            </p>
          </div>
          <Switch
            id="task-sleep-inhibition"
            checked={state.draft}
            disabled={!state.canEdit}
            data-testid="sleep-inhibition-switch"
            data-settings-dirty={state.isDirty}
            onCheckedChange={state.setDraft}
            className="shrink-0 cursor-pointer"
          />
        </div>

        <div className="rounded-md border border-border/70 bg-muted/20 p-3 text-sm">
          <p data-testid="sleep-inhibition-status">
            {t("settings:sleepInhibitionStatusLabel")}: {t(statusMessageKey(snapshot))}
          </p>
          <p className="mt-2 text-xs text-muted-foreground">
            {t("settings:sleepInhibitionCaveat")}
          </p>
        </div>

        {!state.isAdmin && (
          <p className="text-sm text-muted-foreground">{t("settings:sleepInhibitionAdminOnly")}</p>
        )}
        {state.saveFailed && (
          <Alert variant="destructive">
            <IconAlertCircle className="size-4" />
            <AlertDescription>{t("settings:sleepInhibitionSaveFailed")}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </SettingsCard>
  );
}

export function SleepInhibitionSettings() {
  const { t } = useTranslation();
  const state = useSleepInhibitionState();

  if (state.loading && !state.snapshot) {
    return (
      <SettingsCard data-testid="sleep-inhibition-settings">
        <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <Spinner className="size-4" />
          {t("settings:sleepInhibitionLoading")}
        </CardContent>
      </SettingsCard>
    );
  }
  if (state.loadFailed && !state.snapshot) {
    return <SleepInhibitionLoadError onRetry={() => void state.reload()} />;
  }
  return <SleepInhibitionCard state={state} />;
}
