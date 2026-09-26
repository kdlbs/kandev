"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { ModelCombobox } from "@/components/settings/model-combobox";
import {
  FallbackOptionHelp,
  ModelFallbackSettingsShell,
} from "@/components/settings/model-fallback-settings-shell";

function ExactModelOption({
  requireExactModel,
  onChange,
}: {
  requireExactModel: boolean;
  onChange: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-1">
          <Label>{t("settings:requireExactModel")}</Label>
          <FallbackOptionHelp kind="strict" />
        </div>
        <Switch
          checked={requireExactModel}
          onCheckedChange={onChange}
          aria-label={t("settings:requireExactModel")}
        />
      </div>
      <p className="text-xs text-muted-foreground">{t("settings:requireExactModelHelper")}</p>
    </div>
  );
}

function ExplicitFallbackOption({
  availableModels,
  fallbackModel,
  fallbackModelGone,
  autoFallback,
  requireExactModel,
  currentModelId,
  onFallbackModelChange,
}: {
  availableModels: { id: string; name: string }[];
  fallbackModel: string;
  fallbackModelGone: boolean;
  autoFallback: boolean;
  requireExactModel: boolean;
  currentModelId: string | undefined;
  onFallbackModelChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [optedIn, setOptedIn] = useState(false);
  const fallbackEnabled = Boolean(fallbackModel) || optedIn;
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-1">
          <Label className={fallbackModelGone ? "text-destructive" : undefined}>
            {t("settings:agentFallback")}
          </Label>
          <FallbackOptionHelp kind="explicit" />
        </div>
        <Switch
          checked={fallbackEnabled}
          disabled={autoFallback || requireExactModel}
          onCheckedChange={(checked) => {
            setOptedIn(checked);
            if (!checked) onFallbackModelChange("");
          }}
          aria-label={t("settings:agentFallback")}
        />
      </div>
      {fallbackEnabled && (
        <ModelCombobox
          value={fallbackModel}
          onChange={onFallbackModelChange}
          models={
            fallbackModelGone
              ? [
                  ...availableModels,
                  {
                    id: fallbackModel,
                    name: `${fallbackModel} (${t("settings:startModelUnavailable")})`,
                    disabled: true,
                  },
                ]
              : availableModels
          }
          currentModelId={currentModelId}
          placeholder={t("settings:agentFallbackPlaceholder")}
          disabled={autoFallback || requireExactModel}
        />
      )}
      <p className="text-xs text-muted-foreground">
        {autoFallback || requireExactModel
          ? t("settings:agentFallbackDisabledHelper")
          : t("settings:agentFallbackHelper")}
      </p>
    </div>
  );
}

// ModelFallbackFields renders the no-silent-model-fallback controls for the
// inline CLI profile editor inside the shared fallback-settings disclosure.
export function ModelFallbackFields({
  availableModels,
  fallbackModel,
  fallbackModelGone,
  autoFallback,
  requireExactModel,
  currentModelId,
  onFallbackModelChange,
  onAutoFallbackChange,
  onRequireExactModelChange,
}: {
  availableModels: { id: string; name: string }[];
  fallbackModel: string;
  fallbackModelGone: boolean;
  autoFallback: boolean;
  requireExactModel: boolean;
  currentModelId: string | undefined;
  onFallbackModelChange: (v: string) => void;
  onAutoFallbackChange: (v: boolean) => void;
  onRequireExactModelChange: (v: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <ModelFallbackSettingsShell
      autoFallback={autoFallback}
      requireExactModel={requireExactModel}
      strictOption={
        <ExactModelOption
          requireExactModel={requireExactModel}
          onChange={onRequireExactModelChange}
        />
      }
      fallbackModel={fallbackModel}
      automaticOption={
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-1">
              <Label>{t("settings:autoFallback")}</Label>
              <FallbackOptionHelp kind="automatic" />
            </div>
            <Switch
              checked={autoFallback}
              disabled={requireExactModel}
              onCheckedChange={onAutoFallbackChange}
              aria-label={t("settings:autoFallback")}
            />
          </div>
          <p className="text-xs text-muted-foreground">{t("settings:autoFallbackHelper")}</p>
        </div>
      }
      explicitOption={
        <ExplicitFallbackOption
          availableModels={availableModels}
          fallbackModel={fallbackModel}
          fallbackModelGone={fallbackModelGone}
          autoFallback={autoFallback}
          requireExactModel={requireExactModel}
          currentModelId={currentModelId}
          onFallbackModelChange={onFallbackModelChange}
        />
      }
    />
  );
}
