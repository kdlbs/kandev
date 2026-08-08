"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { ModelCombobox } from "@/components/settings/model-combobox";

// ModelFallbackFields renders the no-silent-model-fallback controls for the
// inline CLI profile editor: the "fallback automatically to next model"
// toggle, then the optional explicit fallback model with its attached switch.
// The fallback selector reuses the start model/mode row's grid so both
// selectors render at the same width.
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
    <>
      <div className="flex items-center justify-between gap-3">
        <div className="space-y-0.5">
          <Label>{t("settings:autoFallback")}</Label>
          <p className="text-xs text-muted-foreground">{t("settings:autoFallbackHelper")}</p>
        </div>
        <Switch
          checked={autoFallback}
          onCheckedChange={onAutoFallbackChange}
          aria-label={t("settings:autoFallback")}
        />
      </div>
      {!autoFallback && (
        <div>
          <div className="flex items-center justify-between gap-3">
            <Label className={fallbackModelGone ? "text-destructive" : undefined}>
              {t("settings:agentFallback")}
            </Label>
            <Switch
              checked={fallbackEnabled}
              onCheckedChange={(checked) => {
                setOptedIn(checked);
                if (!checked) onFallbackModelChange("");
              }}
              aria-label={t("settings:agentFallback")}
            />
          </div>
          {fallbackEnabled && (
            <>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
                          },
                        ]
                      : availableModels
                  }
                  currentModelId={currentModelId}
                  placeholder={t("settings:agentFallbackPlaceholder")}
                />
              </div>
              <p className="text-xs text-muted-foreground mt-1">
                {t("settings:agentFallbackHelper")}
              </p>
            </>
          )}
        </div>
      )}
    </>
  );
}
