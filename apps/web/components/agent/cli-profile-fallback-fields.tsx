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

// ModelFallbackFields renders the no-silent-model-fallback controls for the
// inline CLI profile editor inside the shared fallback-settings disclosure.
export function ModelFallbackFields({
  availableModels,
  fallbackModel,
  fallbackModelGone,
  autoFallback,
  currentModelId,
  onFallbackModelChange,
  onAutoFallbackChange,
}: {
  availableModels: { id: string; name: string }[];
  fallbackModel: string;
  fallbackModelGone: boolean;
  autoFallback: boolean;
  currentModelId: string | undefined;
  onFallbackModelChange: (v: string) => void;
  onAutoFallbackChange: (v: boolean) => void;
}) {
  const { t } = useTranslation();
  // Checking the attached switch with no value yet must keep the selector
  // visible until a model is picked, so a transient local flag extends the
  // value-derived enabled state.
  const [optedIn, setOptedIn] = useState(false);
  const fallbackEnabled = Boolean(fallbackModel) || optedIn;
  return (
    <ModelFallbackSettingsShell
      autoFallback={autoFallback}
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
              onCheckedChange={onAutoFallbackChange}
              aria-label={t("settings:autoFallback")}
            />
          </div>
          <p className="text-xs text-muted-foreground">{t("settings:autoFallbackHelper")}</p>
        </div>
      }
      explicitOption={
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
              disabled={autoFallback}
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
              disabled={autoFallback}
            />
          )}
          <p className="text-xs text-muted-foreground">
            {autoFallback
              ? t("settings:agentFallbackDisabledHelper")
              : t("settings:agentFallbackHelper")}
          </p>
        </div>
      }
    />
  );
}
