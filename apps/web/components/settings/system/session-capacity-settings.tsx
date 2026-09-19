"use client";

import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconAlertCircle, IconInfoCircle, IconLock } from "@tabler/icons-react";
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

function SessionCapacityLoadError({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <SettingsCard data-testid="session-capacity-settings">
      <CardContent className="space-y-3 py-6">
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
      </CardContent>
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
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0 flex-1 space-y-1">
        <Label
          htmlFor="session-capacity-enabled"
          className="inline-flex min-h-11 items-center py-2"
        >
          {label}
        </Label>
        <p className="text-sm text-muted-foreground">
          {t("system:sessionCapacityEnabledDescription")}
        </p>
      </div>
      <div
        data-testid="session-capacity-enabled-touch-target"
        className="flex min-h-11 min-w-11 shrink-0 items-center justify-center"
      >
        <Switch
          id="session-capacity-enabled"
          data-testid="session-capacity-enabled"
          checked={checked}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={label}
          className="cursor-pointer [@media(pointer:coarse)]:after:-inset-y-3.5 disabled:cursor-not-allowed"
        />
      </div>
    </div>
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
  const describedBy = error
    ? "session-capacity-maximum-help session-capacity-maximum-error"
    : "session-capacity-maximum-help";
  return (
    <div className="min-w-0 space-y-2 border-t border-border/70 pt-5">
      <Label htmlFor="session-capacity-maximum">{t("system:sessionCapacityMaximumLabel")}</Label>
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
        aria-describedby={describedBy}
        onChange={(event) => onChange(event.target.value)}
        className={settingsControlClassName("w-full max-w-xs")}
      />
      <p id="session-capacity-maximum-help" className="text-xs text-muted-foreground">
        {t("system:sessionCapacityMaximumHelp")}
      </p>
      {error && (
        <p
          id="session-capacity-maximum-error"
          data-testid="session-capacity-maximum-error"
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
    <div className="rounded-md border border-border/70 bg-muted/20 p-3 text-sm">
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
      <p className="mt-2 text-xs text-muted-foreground">{t("system:sessionCapacityScope")}</p>
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

export function SessionCapacitySettings() {
  const { t } = useTranslation();
  const state = useSessionCapacitySettings();

  if (state.loading && !state.snapshot) {
    return (
      <SettingsCard data-testid="session-capacity-settings">
        <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <Spinner className="size-4" />
          {t("system:sessionCapacityLoading")}
        </CardContent>
      </SettingsCard>
    );
  }
  if (state.loadFailed && !state.snapshot) {
    return <SessionCapacityLoadError onRetry={() => void state.reload()} />;
  }
  if (!state.snapshot) return null;

  const { effective, settings } = state.snapshot;
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

  return (
    <SettingsCard
      isDirty={state.isDirty}
      className="min-w-0 w-full"
      data-testid="session-capacity-settings"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("system:sessionCapacityLimitTitle")}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t("system:sessionCapacityLimitDescription")}
        </p>
      </CardHeader>
      <CardContent className="min-w-0 space-y-5">
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
        <div className="flex gap-2 rounded-md border border-border/70 bg-muted/20 p-3 text-sm text-muted-foreground">
          <IconInfoCircle className="size-4 shrink-0" />
          <p>{t("system:sessionCapacityBehaviorHelp")}</p>
        </div>
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
    </SettingsCard>
  );
}
