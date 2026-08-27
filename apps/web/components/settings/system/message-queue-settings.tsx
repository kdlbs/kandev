"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconAlertCircle, IconInfoCircle, IconLock } from "@tabler/icons-react";
import type { TFunction } from "i18next";
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

/** Parses the max-per-session draft text into a non-negative safe integer,
 * or `null` when the text isn't a valid whole number. */
function parseMaximum(value: string): number | null {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

/** Maps an effective-settings source enum to its i18n label key. */
function sourceLabelKey(source: MessageQueueSettingsSource): string {
  switch (source) {
    case "setting":
      return "system:messageQueueSourceSetting";
    case "configuration":
      return "system:messageQueueSourceConfiguration";
    case "environment":
      return "system:messageQueueSourceEnvironment";
    default:
      return "system:messageQueueSourceDefault";
  }
}

function queueCapacityLockMessage(
  t: TFunction,
  source: MessageQueueSettingsSource | undefined,
): string {
  if (source === "configuration") return t("system:messageQueueConfigurationLocked");
  return t("system:messageQueueEnvironmentLocked", { variable: ENVIRONMENT_VARIABLE });
}

type QueueSettingsValidationOptions = {
  parsed: number | null;
  isAdmin: boolean;
  isLocked: boolean;
  isMaxDirty: boolean;
  lockSource: MessageQueueSettingsSource | undefined;
};

function queueSettingsInvalidReason(
  t: TFunction,
  { parsed, isAdmin, isLocked, isMaxDirty, lockSource }: QueueSettingsValidationOptions,
): string | undefined {
  if (parsed === null) return t("system:messageQueueValidation");
  if (!isAdmin) return t("system:messageQueueAdminOnly");
  if (isLocked && isMaxDirty) return queueCapacityLockMessage(t, lockSource);
  return undefined;
}

/** Loads the persisted snapshot and owns the editable queue-setting drafts.
 * Split out of
 * `useMessageQueueSettingsDraft` so neither function needs to grow past the
 * project's per-function line limit as the page gains fields. */
function useMessageQueueSettingsLoad() {
  const [snapshot, setSnapshot] = useState<MessageQueueSettingsResponse | null>(null);
  const [draft, setDraft] = useState("");
  const [mergeDraft, setMergeDraft] = useState(true);
  const [autoMergeDraft, setAutoMergeDraft] = useState(true);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
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
      setMergeDraft(response.settings.merge_enabled);
      setAutoMergeDraft(response.settings.auto_merge_enabled);
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

  return {
    snapshot,
    setSnapshot,
    draft,
    setDraft,
    mergeDraft,
    setMergeDraft,
    autoMergeDraft,
    setAutoMergeDraft,
    loading,
    loadFailed,
    reload,
  };
}

type QueueSettingsContributorOptions = {
  load: ReturnType<typeof useMessageQueueSettingsLoad>;
  parsed: number | null;
  isMaxDirty: boolean;
  isMergeDirty: boolean;
  isAutoMergeDirty: boolean;
  isAdmin: boolean;
  isLocked: boolean;
  invalidReason: string | undefined;
  onSaveFailed: (failed: boolean) => void;
};

/** Registers all queue drafts as one atomic settings-page contributor. */
function useQueueSettingsContributor({
  load,
  parsed,
  isMaxDirty,
  isMergeDirty,
  isAutoMergeDirty,
  isAdmin,
  isLocked,
  invalidReason,
  onSaveFailed,
}: QueueSettingsContributorOptions) {
  const {
    snapshot,
    setSnapshot,
    draft,
    setDraft,
    mergeDraft,
    setMergeDraft,
    autoMergeDraft,
    setAutoMergeDraft,
  } = load;
  const baseline = snapshot?.settings.max_per_session;
  const mergeBaseline = snapshot?.settings.merge_enabled;
  const autoMergeBaseline = snapshot?.settings.auto_merge_enabled;
  const isDirty = isMaxDirty || isMergeDirty || isAutoMergeDirty;
  useSettingsSaveContributor({
    id: "system-message-queue",
    revision: `${draft}:${mergeDraft}:${autoMergeDraft}`,
    isDirty,
    canSave: parsed !== null && isAdmin && (!isMaxDirty || !isLocked),
    invalidReason,
    save: async () => {
      if (parsed === null || !isAdmin || (isMaxDirty && isLocked)) {
        throw new Error(invalidReason);
      }
      const submitted = draft;
      const submittedMerge = mergeDraft;
      const submittedAutoMerge = autoMergeDraft;
      onSaveFailed(false);
      try {
        const response = await updateMessageQueueSettings({
          ...(isMaxDirty ? { max_per_session: parsed } : {}),
          ...(isMergeDirty ? { merge_enabled: mergeDraft } : {}),
          ...(isAutoMergeDirty ? { auto_merge_enabled: autoMergeDraft } : {}),
        });
        setSnapshot(response);
        setDraft((current) =>
          current === submitted ? String(response.settings.max_per_session) : current,
        );
        setMergeDraft((current) =>
          current === submittedMerge ? response.settings.merge_enabled : current,
        );
        setAutoMergeDraft((current) =>
          current === submittedAutoMerge ? response.settings.auto_merge_enabled : current,
        );
      } catch (error) {
        onSaveFailed(true);
        throw error;
      }
    },
    discard: () => {
      if (baseline !== undefined) setDraft(String(baseline));
      if (mergeBaseline !== undefined) setMergeDraft(mergeBaseline);
      if (autoMergeBaseline !== undefined) setAutoMergeDraft(autoMergeBaseline);
      onSaveFailed(false);
    },
  });
}

/** Derives dirty, validation, and permission state for all queue drafts. */
function useMessageQueueSettingsDraft() {
  const { t } = useTranslation();
  const role = useAppStore((state) => state.auth.user?.role);
  const [saveFailed, setSaveFailed] = useState(false);
  const load = useMessageQueueSettingsLoad();
  const { snapshot, draft, mergeDraft, autoMergeDraft } = load;
  const parsed = parseMaximum(draft);
  const baseline = snapshot?.settings.max_per_session;
  const mergeBaseline = snapshot?.settings.merge_enabled;
  const autoMergeBaseline = snapshot?.settings.auto_merge_enabled;
  const isMaxDirty = baseline !== undefined && draft !== String(baseline);
  const isMergeDirty = mergeBaseline !== undefined && mergeDraft !== mergeBaseline;
  const isAutoMergeDirty = autoMergeBaseline !== undefined && autoMergeDraft !== autoMergeBaseline;
  const isAdmin = role === undefined || role === "admin";
  const isLocked = snapshot?.effective.locked === true;
  const lockSource = snapshot?.effective.source;
  const invalidReason = queueSettingsInvalidReason(t, {
    parsed,
    isAdmin,
    isLocked,
    isMaxDirty,
    lockSource,
  });

  useQueueSettingsContributor({
    load,
    parsed,
    isMaxDirty,
    isMergeDirty,
    isAutoMergeDirty,
    isAdmin,
    isLocked,
    invalidReason,
    onSaveFailed: setSaveFailed,
  });

  return {
    snapshot,
    draft,
    setDraft: load.setDraft,
    mergeDraft,
    setMergeDraft: load.setMergeDraft,
    autoMergeDraft,
    setAutoMergeDraft: load.setAutoMergeDraft,
    loading: load.loading,
    loadFailed: load.loadFailed,
    saveFailed,
    isDirty: isMaxDirty || isMergeDirty || isAutoMergeDirty,
    isAdmin,
    isLocked,
    lockSource,
    reload: load.reload,
  };
}

type QueueLimitFieldsProps = {
  draft: string;
  onDraftChange: (value: string) => void;
  disabled: boolean;
  configured: number;
  effectiveValue: string;
  source: MessageQueueSettingsSource;
};

/** Renders the per-session limit input plus the configured/effective summary
 * badge; a pure presentational block extracted so `MessageQueueSettings`
 * stays under the per-function line limit. */
function QueueLimitFields({
  draft,
  onDraftChange,
  disabled,
  configured,
  effectiveValue,
  source,
}: QueueLimitFieldsProps) {
  const { t } = useTranslation();
  return (
    <>
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
          value={draft}
          disabled={disabled}
          onChange={(event) => onDraftChange(event.target.value)}
          className="h-11 w-full max-w-xs"
        />
        <p className="text-xs text-muted-foreground">{t("system:messageQueueUnlimitedHelp")}</p>
      </div>
      <div className="rounded-md border border-border/70 bg-muted/20 p-3 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground">{t("system:messageQueueConfigured")}</span>
          <span>{configured}</span>
          <span className="text-muted-foreground">{t("system:messageQueueEffective")}</span>
          <strong data-testid="message-queue-effective-value">{effectiveValue}</strong>
          <Badge variant="secondary" data-testid="message-queue-source">
            {t(sourceLabelKey(source))}
          </Badge>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          {t("system:messageQueueEffectiveHelp")}
        </p>
      </div>
    </>
  );
}

type MergeToggleFieldsProps = {
  enabled: boolean;
  onChange: (next: boolean) => void;
  disabled: boolean;
};

/** Renders the merge-enabled switch plus its limitations notice; extracted
 * for the same per-function line-limit reason as `QueueLimitFields`. */
function MergeToggleFields({ enabled, onChange, disabled }: MergeToggleFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-3 border-t border-border/70 pt-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <Label htmlFor="message-queue-merge-enabled">
            {t("system:messageQueueMergeToggleLabel")}
          </Label>
          <p className="text-sm text-muted-foreground">
            {t("system:messageQueueMergeDescription")}
          </p>
        </div>
        <Switch
          id="message-queue-merge-enabled"
          data-testid="message-queue-merge-enabled"
          checked={enabled}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={t("system:messageQueueMergeToggleLabel")}
          className="cursor-pointer disabled:cursor-not-allowed"
        />
      </div>
      <div className="flex gap-2 rounded-md border border-border/70 bg-muted/20 p-3 text-sm text-muted-foreground">
        <IconInfoCircle className="size-4 shrink-0" />
        <p>{t("system:messageQueueMergeNotice")}</p>
      </div>
    </div>
  );
}

/** Renders the automatic same-source admission merge setting. */
function AutoMergeToggleFields({ enabled, onChange, disabled }: MergeToggleFieldsProps) {
  const { t } = useTranslation();
  const label = t("system:messageQueueAutoMergeToggleLabel");
  return (
    <div className="min-w-0 space-y-3 border-t border-border/70 pt-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1">
          <Label
            htmlFor="message-queue-auto-merge-enabled"
            className="inline-flex min-h-11 cursor-pointer items-center py-2"
          >
            {label}
          </Label>
          <p className="text-sm text-muted-foreground">
            {t("system:messageQueueAutoMergeDescription")}
          </p>
        </div>
        <div
          data-testid="message-queue-auto-merge-touch-target"
          className="flex min-h-11 min-w-11 shrink-0 items-center justify-center"
        >
          <Switch
            id="message-queue-auto-merge-enabled"
            data-testid="message-queue-auto-merge-enabled"
            checked={enabled}
            disabled={disabled}
            onCheckedChange={onChange}
            aria-label={label}
            className="cursor-pointer [@media(pointer:coarse)]:after:-inset-y-3.5 disabled:cursor-not-allowed"
          />
        </div>
      </div>
      <div className="flex gap-2 rounded-md border border-border/70 bg-muted/20 p-3 text-sm text-muted-foreground">
        <IconInfoCircle className="size-4 shrink-0" />
        <p>{t("system:messageQueueAutoMergeNotice")}</p>
      </div>
    </div>
  );
}

/** Settings → Task Behavior → Message Queue controls, sharing one save contributor. */
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
  const isConfigurationLocked = state.lockSource === "configuration";
  const effectiveValue =
    effective.max_per_session === 0
      ? t("system:messageQueueUnlimited")
      : String(effective.max_per_session);

  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full"
      data-testid="message-queue-settings"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("system:messageQueueLimitTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="min-w-0 space-y-5">
        <p className="text-sm text-muted-foreground">{t("system:messageQueueLimitDescription")}</p>
        <QueueLimitFields
          draft={state.draft}
          onDraftChange={state.setDraft}
          disabled={!state.isAdmin || state.isLocked}
          configured={settings.max_per_session}
          effectiveValue={effectiveValue}
          source={effective.source}
        />

        {state.isLocked && (
          <Alert>
            <IconLock className="size-4" />
            <AlertTitle>
              {t(
                isConfigurationLocked
                  ? "system:messageQueueConfigurationLockTitle"
                  : "system:messageQueueEnvironmentLockTitle",
              )}
            </AlertTitle>
            <AlertDescription>{queueCapacityLockMessage(t, state.lockSource)}</AlertDescription>
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

        <MergeToggleFields
          enabled={state.mergeDraft}
          onChange={state.setMergeDraft}
          disabled={!state.isAdmin}
        />
        <AutoMergeToggleFields
          enabled={state.autoMergeDraft}
          onChange={state.setAutoMergeDraft}
          disabled={!state.isAdmin}
        />
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
