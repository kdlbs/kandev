"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { IconFlask, IconLock, IconRefresh } from "@tabler/icons-react";
import type { TFunction } from "i18next";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { SettingsCard } from "@/components/settings/settings-card";

type FeatureToggleCardProps = {
  flag: RuntimeFlagState;
  isDirty?: boolean;
  saving: boolean;
  onChange: (next: boolean) => void;
  onReset: () => void;
};

export function FeatureToggleCard({
  flag,
  isDirty = false,
  saving,
  onChange,
  onReset,
}: FeatureToggleCardProps) {
  const { t } = useTranslation();
  const disabled = saving || flag.env_locked || !flag.mutable;
  return (
    <SettingsCard isDirty={isDirty} data-testid={`feature-toggle-${flag.key}`}>
      <CardHeader className="gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-2">
          {/* `label`, `description` and `risk_description` are authored in
              runtimeflags/registry.go and rendered by the backend, so they stay
              English here — localizing them needs a key/value split in Go. */}
          <CardTitle className="flex flex-wrap items-center gap-2 text-base">
            {flag.label}
            <FlagBadges flag={flag} />
          </CardTitle>
          <p className="text-sm text-muted-foreground">{flag.description}</p>
        </div>
        <Switch
          checked={flag.effective_value}
          data-settings-dirty={isDirty}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={t("system:featureToggleSwitchLabel", { label: flag.label })}
          className="cursor-pointer disabled:cursor-not-allowed"
        />
      </CardHeader>
      <CardContent className="space-y-3">
        {flag.risk_description && (
          <p className="text-sm leading-6 text-muted-foreground">{flag.risk_description}</p>
        )}
        <FlagMetadata flag={flag} />
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <Button
            variant="outline"
            size="sm"
            disabled={saving || flag.env_locked || !flag.mutable || flag.override_value == null}
            onClick={onReset}
            className="cursor-pointer disabled:cursor-not-allowed"
          >
            <IconRefresh className="mr-1 h-3.5 w-3.5" />
            {t("system:featureToggleUseDefault")}
          </Button>
          {flag.env_locked && (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <IconLock className="h-3.5 w-3.5" />
              {t("system:featureToggleEnvControlled")}
            </span>
          )}
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function FlagBadges({ flag }: { flag: RuntimeFlagState }) {
  const { t } = useTranslation();
  return (
    <>
      {flag.stability === "experimental" && (
        <Badge variant="secondary" className="gap-1">
          <IconFlask className="h-3 w-3" />
          {t("system:featureToggleExperimental")}
        </Badge>
      )}
      {flag.kind === "debug" && <Badge variant="outline">{t("system:featureToggleDebug")}</Badge>}
    </>
  );
}

function FlagMetadata({ flag }: { flag: RuntimeFlagState }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 text-xs text-muted-foreground sm:flex-row sm:flex-wrap sm:items-center">
      <span>{t("system:featureToggleSource", { source: sourceLabel(flag, t) })}</span>
      {/* `env_var` is the process environment variable name — an identifier. */}
      <span>{t("system:featureToggleEnv", { envVar: flag.env_var })}</span>
      {flag.restart_required && <span>{t("system:featureToggleRequiresRestart")}</span>}
      {flag.requires_restart_to_apply && (
        <span className="font-medium text-amber-700">
          {t("system:featureTogglePendingRestart")}
        </span>
      )}
    </div>
  );
}

/** `source` is a wire enum; only its label is copy. */
function sourceLabel(flag: RuntimeFlagState, t: TFunction): string {
  if (flag.source === "env") return t("system:featureToggleSourceEnv");
  if (flag.source === "override") return t("system:featureToggleSourceOverride");
  return t("system:featureToggleSourceDefault");
}
