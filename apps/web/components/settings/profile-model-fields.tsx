"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { ModeCombobox } from "@/components/settings/mode-combobox";
import {
  configOptionToModelOptions,
  isModelConfigOption,
  ModelConfigSelector,
  type ModelSelectorOption,
  type SelectConfigOption,
  usableConfigOptions,
} from "@/components/model-config-selector";
import {
  profileAutoFallbackIsDirty,
  profileFallbackModelIsDirty,
} from "@/components/settings/profile-capability-helpers";
import type { ModelConfig, ModeEntry, ModelEntry } from "@/lib/types/http";
import type { PermissionKey } from "@/lib/agent-permissions";
import type { CLIFlag } from "@/lib/types/http";

export type ProfileFormData = {
  name: string;
  model: string;
  /** Optional single fallback model applied when `model` is unavailable. */
  fallback_model?: string;
  /** Legacy automatic-fallback opt-in; hides the fallback_model field. */
  auto_fallback?: boolean;
  mode: string;
  config_options?: Record<string, string>;
  cli_passthrough: boolean;
  cli_flags: CLIFlag[];
  command_prefix?: string;
} & Record<PermissionKey, boolean>;

export function ModelPicker({
  profile,
  models,
  currentModelId,
  configOptions,
  onChange,
  ariaLabel,
  placeholder,
  goneModelLabel,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  currentModelId: string | undefined;
  configOptions: SelectConfigOption[];
  onChange: (patch: Partial<ProfileFormData>) => void;
  ariaLabel: string;
  placeholder?: string;
  goneModelLabel: string;
}) {
  const { t } = useTranslation();
  const modelConfig = configOptions.find(isModelConfigOption);
  const modelOptions: ModelSelectorOption[] = modelConfig
    ? configOptionToModelOptions(modelConfig)
    : models.map((model) => ({
        id: model.id,
        name: model.name,
        description: model.description || (model.id !== model.name ? model.id : undefined),
        usageMultiplier:
          typeof model.meta?.copilotUsage === "string" ? model.meta.copilotUsage : undefined,
      }));
  const currentModel = profile.model || modelConfig?.currentValue || currentModelId || null;
  // A configured model that is no longer advertised ("gone") stays visible
  // in the list, greyed out and unselectable, so the user sees what was
  // configured instead of it silently vanishing.
  const modelIsGone = Boolean(profile.model && !modelOptions.some((m) => m.id === profile.model));
  if (modelIsGone) {
    modelOptions.unshift({
      id: profile.model!,
      name: profile.model!,
      disabled: true,
      disabledReason: goneModelLabel,
    });
  }
  const selectedConfigOptions = configOptions.map((option) => ({
    ...option,
    currentValue: isModelConfigOption(option)
      ? profile.model || option.currentValue
      : profile.config_options?.[option.id] || option.currentValue,
  }));

  return (
    <ModelConfigSelector
      modelOptions={modelOptions}
      currentModel={currentModel}
      configOptions={selectedConfigOptions}
      onModelChange={(value) => onChange({ model: value })}
      onConfigChange={(configId, value) =>
        onChange({ config_options: { ...(profile.config_options ?? {}), [configId]: value } })
      }
      placeholder={placeholder ?? t("settings:selectAModel")}
      ariaLabel={ariaLabel}
      triggerClassName={modelIsGone ? "text-destructive" : undefined}
    />
  );
}

export function ModePicker({
  profile,
  modes,
  currentModeId,
  onChange,
}: {
  profile: ProfileFormData;
  modes: ModeEntry[];
  currentModeId: string | undefined;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  return (
    <ModeCombobox
      value={profile.mode}
      onChange={(value) => onChange({ mode: value })}
      modes={modes}
      currentModeId={currentModeId}
    />
  );
}

export function modelConfigOptions(modelConfig: ModelConfig): SelectConfigOption[] {
  return usableConfigOptions(
    modelConfig.config_options?.map((option) => ({
      type: option.type,
      id: option.id,
      name: option.name,
      currentValue: option.current_value,
      category: option.category,
      options: option.options,
    })),
  );
}

// ModelFallbackSection renders the no-silent-model-fallback controls: the
// "fallback automatically to next model" toggle on top, and below it the
// optional explicit fallback model (hidden while auto-fallback is on). The
// fallback model is itself optional, so its row carries an attached switch:
// checking it enables the model selector and records fallback_model;
// unchecking clears the value (strict mode). The explicit fallback is the
// only automatic model switch allowed when auto-fallback is off.
export function ModelFallbackSection({
  profile,
  models,
  currentModelId,
  configOptions,
  baselineProfile,
  labelCls,
  gapCls,
  alignSelectorWithStart,
  onChange,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  currentModelId: string | undefined;
  configOptions: SelectConfigOption[];
  baselineProfile?: ProfileFormData;
  labelCls?: string;
  gapCls: string;
  /** Constrain the fallback selector to the same width as the start-model
   *  cell (which sits next to the mode cell) instead of spanning the row. */
  alignSelectorWithStart?: boolean;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  // The fallback model is optional: its attached switch tracks whether the
  // user opted in. Checking with no value yet must keep the selector visible
  // (empty) until a model is picked, so a transient local flag extends the
  // value-derived state.
  const [optedIn, setOptedIn] = useState(false);
  const fallbackEnabled = Boolean(profile.fallback_model) || optedIn;
  const fallbackSelector = (
    <ModelPicker
      profile={{ ...profile, model: profile.fallback_model ?? "" }}
      models={models}
      currentModelId={currentModelId}
      configOptions={configOptions}
      onChange={(patch) => onChange({ fallback_model: patch.model })}
      ariaLabel={t("settings:agentFallbackAria")}
      placeholder={t("settings:agentFallbackPlaceholder")}
      goneModelLabel={t("settings:fallbackModelUnavailable")}
    />
  );
  return (
    <>
      <div
        data-testid="profile-auto-fallback-field"
        className="flex items-start justify-between gap-3"
        data-settings-dirty={profileAutoFallbackIsDirty(profile, baselineProfile)}
        data-settings-dirty-level="container"
      >
        <div className={`min-w-0 ${gapCls}`}>
          <Label className={labelCls}>{t("settings:autoFallback")}</Label>
          <p className="text-xs text-muted-foreground">{t("settings:autoFallbackHelper")}</p>
        </div>
        <Switch
          checked={profile.auto_fallback ?? false}
          onCheckedChange={(checked) => onChange({ auto_fallback: checked })}
          aria-label={t("settings:autoFallback")}
        />
      </div>
      {!profile.auto_fallback && (
        <div
          data-testid="profile-fallback-model-field"
          className={gapCls}
          data-settings-dirty={profileFallbackModelIsDirty(profile, baselineProfile)}
          data-settings-dirty-level="container"
        >
          <div className="flex items-start justify-between gap-3">
            <Label className={labelCls}>{t("settings:agentFallback")}</Label>
            <Switch
              checked={fallbackEnabled}
              onCheckedChange={(checked) => {
                setOptedIn(checked);
                if (!checked) onChange({ fallback_model: "" });
              }}
              aria-label={t("settings:agentFallback")}
            />
          </div>
          {fallbackEnabled && (
            <div className={alignSelectorWithStart ? "flex items-end gap-2" : undefined}>
              <div className={alignSelectorWithStart ? "flex-1 min-w-0" : undefined}>
                {fallbackSelector}
              </div>
              {alignSelectorWithStart && <div className="flex-1 min-w-0" aria-hidden />}
            </div>
          )}
          {fallbackEnabled && (
            <p className="text-xs text-muted-foreground">{t("settings:agentFallbackHelper")}</p>
          )}
        </div>
      )}
    </>
  );
}
