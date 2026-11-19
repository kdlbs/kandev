"use client";

import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { SettingsRow } from "../settings-group";
import { SettingsInfo } from "../settings-info";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconAlertCircle, IconLock } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { SettingsCard } from "@/components/settings/settings-card";
import {
  settingsActionClassName,
  settingsControlClassName,
} from "@/components/settings/settings-control";
import {
  queueCapacityLockMessage,
  useMessageQueueSettingsDraft,
} from "./use-message-queue-settings-draft";
import type { MessageQueueSettingsSource } from "@/lib/types/system";

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
      <SettingsRow
        label={t("system:messageQueueMaximumLabel")}
        description={t("settings:queueMaximumShort")}
        controlId="message-queue-max-per-session"
        info={
          <SettingsInfo label={t("system:messageQueueMaximumLabel")}>
            <p>{t("system:messageQueueLimitDescription")}</p>
            <p>{t("system:messageQueueUnlimitedHelp")}</p>
            <p>{t("system:messageQueueEffectiveHelp")}</p>
          </SettingsInfo>
        }
        control={
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
            className={settingsControlClassName("w-full md:w-40")}
          />
        }
      />
      <div className="text-xs text-muted-foreground">
        <div className="flex flex-wrap items-center gap-2">
          {String(configured) !== effectiveValue && (
            <>
              <span>{t("system:messageQueueConfigured")}</span>
              <span>{configured}</span>
            </>
          )}
          <span className="text-muted-foreground">{t("system:messageQueueEffective")}</span>
          <strong data-testid="message-queue-effective-value">{effectiveValue}</strong>
          <Badge variant="secondary" data-testid="message-queue-source">
            {t(sourceLabelKey(source))}
          </Badge>
        </div>
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
  const label = t("system:messageQueueMergeToggleLabel");
  return (
    <SettingsRow
      label={label}
      description={t("settings:queueMergeShort")}
      controlId="message-queue-merge-enabled"
      touchTarget="switch"
      controlWrapperTestId="message-queue-merge-touch-target"
      info={
        <SettingsInfo label={label}>
          <p>{t("system:messageQueueMergeDescription")}</p>
          <p>{t("system:messageQueueMergeNotice")}</p>
        </SettingsInfo>
      }
      control={
        <Switch
          id="message-queue-merge-enabled"
          data-testid="message-queue-merge-enabled"
          checked={enabled}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={label}
          className="cursor-pointer disabled:cursor-not-allowed"
        />
      }
    />
  );
}

function AutoMergeToggleFields({ enabled, onChange, disabled }: MergeToggleFieldsProps) {
  const { t } = useTranslation();
  const label = t("system:messageQueueAutoMergeToggleLabel");
  return (
    <SettingsRow
      label={label}
      description={t("settings:queueAutoMergeShort")}
      controlId="message-queue-auto-merge-enabled"
      touchTarget="switch"
      controlWrapperTestId="message-queue-auto-merge-touch-target"
      info={
        <SettingsInfo label={label}>
          <p>{t("system:messageQueueAutoMergeDescription")}</p>
          <p>{t("system:messageQueueAutoMergeNotice")}</p>
        </SettingsInfo>
      }
      control={
        <Switch
          id="message-queue-auto-merge-enabled"
          data-testid="message-queue-auto-merge-enabled"
          checked={enabled}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={label}
          className="cursor-pointer disabled:cursor-not-allowed"
        />
      }
    />
  );
}

function MessageQueueLoadingState({ withinGroup }: { withinGroup: boolean }) {
  const { t } = useTranslation();
  const loadingContent = (
    <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
      <Spinner className="size-4" />
      {t("system:messageQueueLoading")}
    </div>
  );
  if (withinGroup) return <div data-testid="message-queue-settings">{loadingContent}</div>;
  return (
    <SettingsCard>
      <CardContent>{loadingContent}</CardContent>
    </SettingsCard>
  );
}

function MessageQueueSettingsReady({
  state,
  withinGroup,
}: MessageQueueSettingsContentProps & {
  state: NonNullable<MessageQueueSettingsContentProps["state"]>;
}) {
  const { t } = useTranslation();
  const { settings, effective } = state.snapshot!;
  const isConfigurationLocked = state.lockSource === "configuration";
  const effectiveValue =
    effective.max_per_session === 0
      ? t("system:messageQueueUnlimited")
      : String(effective.max_per_session);

  const content = (
    <>
      {withinGroup ? (
        <div className="space-y-1 pb-3">
          <h4 className="text-sm font-semibold">{t("system:messageQueueTitle")}</h4>
        </div>
      ) : (
        <CardHeader>
          <CardTitle className="text-base">{t("system:messageQueueLimitTitle")}</CardTitle>
        </CardHeader>
      )}
      <CardContent className={withinGroup ? "min-w-0 space-y-5 px-0" : "min-w-0 space-y-5"}>
        {!withinGroup && (
          <p className="text-sm text-muted-foreground">
            {t("system:messageQueueLimitDescription")}
          </p>
        )}
        <QueueLimitFields
          draft={state.draft}
          onDraftChange={state.setDraft}
          disabled={!state.isAdmin || state.isLocked}
          configured={settings.max_per_session}
          effectiveValue={effectiveValue}
          source={effective.source}
        />

        {state.isDirty && state.invalidReason && (
          <p role="alert" className="text-sm text-destructive">
            {state.invalidReason}
          </p>
        )}
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
    </>
  );

  if (withinGroup) {
    return (
      <div className="min-w-0 py-3" data-testid="message-queue-settings">
        {content}
      </div>
    );
  }

  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full"
      data-testid="message-queue-settings"
    >
      {content}
    </SettingsCard>
  );
}

/** Settings → Task Behavior → Message Queue controls, sharing one save contributor. */
type MessageQueueSettingsContentProps = {
  state: ReturnType<typeof useMessageQueueSettingsDraft>;
  withinGroup?: boolean;
};

export function MessageQueueSettings() {
  const state = useMessageQueueSettingsDraft();
  return <MessageQueueSettingsContent state={state} />;
}

export function MessageQueueSettingsContent({
  state,
  withinGroup = false,
}: MessageQueueSettingsContentProps) {
  if (state.loading && !state.snapshot) {
    return <MessageQueueLoadingState withinGroup={withinGroup} />;
  }

  if (state.loadFailed && !state.snapshot) {
    return <MessageQueueLoadError onRetry={() => void state.reload()} withinGroup={withinGroup} />;
  }

  if (!state.snapshot) return null;
  return <MessageQueueSettingsReady state={state} withinGroup={withinGroup} />;
}

function MessageQueueLoadError({
  onRetry,
  withinGroup = false,
}: {
  onRetry: () => void;
  withinGroup?: boolean;
}) {
  const { t } = useTranslation();
  const content = (
    <div className="space-y-3 py-6">
      <Alert variant="destructive">
        <IconAlertCircle className="size-4" />
        <AlertDescription>{t("system:messageQueueLoadFailed")}</AlertDescription>
      </Alert>
      <Button
        variant="outline"
        className={settingsActionClassName("cursor-pointer")}
        onClick={onRetry}
      >
        {t("system:messageQueueRetry")}
      </Button>
    </div>
  );
  if (withinGroup) return <div data-testid="message-queue-settings">{content}</div>;
  return (
    <SettingsCard>
      <CardContent>{content}</CardContent>
    </SettingsCard>
  );
}
