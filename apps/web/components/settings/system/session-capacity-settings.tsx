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
  sessionCapacitySourceLabelKey,
  useSessionCapacitySettings,
} from "./use-session-capacity-settings";

const ENVIRONMENT_VARIABLE = "KANDEV_MAX_CONCURRENT_SESSIONS";

function SessionCapacityLoadError({
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
        <AlertDescription>{t("system:sessionCapacityLoadFailed")}</AlertDescription>
      </Alert>
      <Button
        variant="outline"
        className={settingsActionClassName("cursor-pointer")}
        onClick={onRetry}
      >
        {t("system:sessionCapacityRetry")}
      </Button>
    </div>
  );
  if (withinGroup) return <div data-testid="session-capacity-settings">{content}</div>;
  return (
    <SettingsCard data-testid="session-capacity-settings">
      <CardContent>{content}</CardContent>
    </SettingsCard>
  );
}

function SessionCapacitySwitch({
  checked,
  disabled,
  onChange,
}: {
  checked: boolean;
  disabled: boolean;
  onChange: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  const label = t("system:sessionCapacityEnabledLabel");
  return (
    <SettingsRow
      label={label}
      description={t("settings:sessionLimitShort")}
      controlId="session-capacity-enabled"
      touchTarget="switch"
      controlWrapperTestId="session-capacity-enabled-touch-target"
      info={
        <SettingsInfo label={label}>
          <p>{t("system:sessionCapacityEnabledDescription")}</p>
          <p>{t("system:sessionCapacityBehaviorHelp")}</p>
        </SettingsInfo>
      }
      control={
        <Switch
          id="session-capacity-enabled"
          data-testid="session-capacity-enabled"
          checked={checked}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={label}
          className="cursor-pointer disabled:cursor-not-allowed"
        />
      }
    />
  );
}

function MaximumField({
  value,
  disabled,
  error,
  onChange,
}: {
  value: string;
  disabled: boolean;
  error?: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <SettingsRow
        label={t("system:sessionCapacityMaximumLabel")}
        description={t("settings:sessionMaximumShort")}
        descriptionId="session-capacity-maximum-help"
        controlId="session-capacity-maximum"
        info={
          <SettingsInfo label={t("system:sessionCapacityMaximumLabel")}>
            {t("system:sessionCapacityMaximumHelp")}
          </SettingsInfo>
        }
        control={
          <Input
            id="session-capacity-maximum"
            data-testid="session-capacity-maximum"
            type="number"
            inputMode="numeric"
            min={1}
            max={2147483647}
            step={1}
            value={value}
            disabled={disabled}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? "session-capacity-maximum-error" : undefined}
            onChange={(event) => onChange(event.target.value)}
            className={settingsControlClassName("w-full md:w-40")}
          />
        }
      />
      {error && (
        <p
          id="session-capacity-maximum-error"
          data-testid="session-capacity-maximum-error"
          role="alert"
          className="text-sm text-destructive"
        >
          {error}
        </p>
      )}
    </div>
  );
}

function EffectiveCapacity({
  enabled,
  maximum,
  source,
  isDirty,
}: {
  enabled: boolean;
  maximum: number;
  source: Parameters<typeof sessionCapacitySourceLabelKey>[0];
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  const current = enabled ? String(maximum) : t("system:sessionCapacityNoLimit");
  return (
    <div className="text-xs text-muted-foreground">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-muted-foreground">{t("system:sessionCapacityCurrent")}</span>
        <strong data-testid="session-capacity-effective-value">{current}</strong>
        <Badge variant="secondary" data-testid="session-capacity-source">
          {t(sessionCapacitySourceLabelKey(source))}
        </Badge>
        {isDirty && (
          <span className="text-muted-foreground">({t("system:sessionCapacityUnsaved")})</span>
        )}
      </div>
    </div>
  );
}

function SessionCapacityManagedNotice() {
  const { t } = useTranslation();
  return (
    <Alert>
      <IconLock className="size-4" />
      <AlertTitle>{t("system:sessionCapacityEnvironmentLockTitle")}</AlertTitle>
      <AlertDescription>
        {t("system:sessionCapacityEnvironmentLocked", { variable: ENVIRONMENT_VARIABLE })}
      </AlertDescription>
    </Alert>
  );
}

function sessionCapacityMaximumError({
  isAdmin,
  isLocked,
  enabled,
  parsed,
  invalidReason,
}: {
  isAdmin: boolean;
  isLocked: boolean;
  enabled: boolean;
  parsed: number | null;
  invalidReason: string | undefined;
}) {
  if (!isAdmin || isLocked || !enabled || parsed !== null) return undefined;
  return invalidReason;
}

function SessionCapacityLoadingState({ withinGroup }: { withinGroup: boolean }) {
  const { t } = useTranslation();
  const loadingContent = (
    <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
      <Spinner className="size-4" />
      {t("system:sessionCapacityLoading")}
    </div>
  );
  if (withinGroup) return <div data-testid="session-capacity-settings">{loadingContent}</div>;
  return (
    <SettingsCard data-testid="session-capacity-settings">
      <CardContent>{loadingContent}</CardContent>
    </SettingsCard>
  );
}

type SessionCapacitySettingsContentProps = {
  state: ReturnType<typeof useSessionCapacitySettings>;
  withinGroup?: boolean;
};

export function SessionCapacitySettings() {
  const state = useSessionCapacitySettings();
  return <SessionCapacitySettingsContent state={state} />;
}

function SessionCapacitySettingsReady({
  state,
  withinGroup,
}: SessionCapacitySettingsContentProps & {
  state: NonNullable<SessionCapacitySettingsContentProps["state"]>;
}) {
  const { t } = useTranslation();
  const { effective, settings } = state.snapshot!;
  const controlsDisabled = !state.isAdmin || state.isLocked;
  const effectiveEnabled = state.isLocked ? effective.enabled : state.enabledDraft;
  const effectiveMaximum = state.isLocked ? effective.max_sessions : settings.max_sessions;
  const maximumError = sessionCapacityMaximumError({
    isAdmin: state.isAdmin,
    isLocked: state.isLocked,
    enabled: state.enabledDraft,
    parsed: state.parsed,
    invalidReason: state.invalidReason,
  });

  const content = (
    <>
      {withinGroup ? (
        <div className="space-y-1 pb-3">
          <h4 className="text-sm font-semibold">{t("system:sessionCapacityLimitTitle")}</h4>
        </div>
      ) : (
        <CardHeader>
          <CardTitle className="text-base">{t("system:sessionCapacityLimitTitle")}</CardTitle>
        </CardHeader>
      )}
      <CardContent className={withinGroup ? "min-w-0 space-y-5 px-0" : "min-w-0 space-y-5"}>
        <SessionCapacitySwitch
          checked={effectiveEnabled}
          disabled={controlsDisabled}
          onChange={state.setEnabledDraft}
        />
        {effectiveEnabled && (
          <MaximumField
            value={state.isLocked ? String(effective.max_sessions) : state.maxDraft}
            disabled={controlsDisabled}
            error={maximumError}
            onChange={state.setMaxDraft}
          />
        )}
        <EffectiveCapacity
          enabled={effective.enabled}
          maximum={effectiveMaximum}
          source={effective.source}
          isDirty={state.isDirty}
        />
        {state.isLocked && <SessionCapacityManagedNotice />}
        {!state.isAdmin && (
          <p className="text-sm text-muted-foreground">{t("system:sessionCapacityAdminOnly")}</p>
        )}
        {state.saveFailed && (
          <Alert variant="destructive">
            <IconAlertCircle className="size-4" />
            <AlertDescription>{t("system:sessionCapacitySaveFailed")}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </>
  );

  if (withinGroup) {
    return (
      <div className="min-w-0 py-3" data-testid="session-capacity-settings">
        {content}
      </div>
    );
  }

  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full"
      data-testid="session-capacity-settings"
    >
      {content}
    </SettingsCard>
  );
}

export function SessionCapacitySettingsContent({
  state,
  withinGroup = false,
}: SessionCapacitySettingsContentProps) {
  if (state.loading && !state.snapshot) {
    return <SessionCapacityLoadingState withinGroup={withinGroup} />;
  }
  if (state.loadFailed && !state.snapshot) {
    return (
      <SessionCapacityLoadError onRetry={() => void state.reload()} withinGroup={withinGroup} />
    );
  }
  if (!state.snapshot) return null;
  return <SessionCapacitySettingsReady state={state} withinGroup={withinGroup} />;
}
