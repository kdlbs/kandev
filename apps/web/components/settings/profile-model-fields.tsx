"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
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
import {
  FallbackOptionHelp,
  ModelFallbackSettingsShell,
} from "@/components/settings/model-fallback-settings-shell";
import type { ModelConfig, ModeEntry, ModelEntry } from "@/lib/types/http";
import type { PermissionKey } from "@/lib/agent-permissions";
import type { CLIFlag } from "@/lib/types/http";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";

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
  disabled,
  configOptionsLoading,
  keepOpenOnModelChange,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  currentModelId: string | undefined;
  configOptions: SelectConfigOption[];
  onChange: (patch: Partial<ProfileFormData>) => void;
  ariaLabel: string;
  placeholder?: string;
  goneModelLabel: string;
  disabled?: boolean;
  configOptionsLoading?: boolean;
  keepOpenOnModelChange?: boolean;
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
      popoverAlign="start"
      disabled={disabled}
      configOptionsLoading={configOptionsLoading}
      keepOpenOnModelChange={keepOpenOnModelChange}
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

// ModelFallbackSection renders the no-silent-model-fallback controls inside a
// collapsed disclosure. The explicit fallback remains visible while automatic
// fallback is enabled, but its controls are disabled and its saved value is
// preserved until automatic fallback is turned off again.
// FallbackModelPicker owns the optional fallback row: its attached switch
// tracks whether the user opted in. Checking with no value yet must keep the
// selector visible (empty) until a model is picked, so a transient local flag
// extends the value-derived state.
function FallbackModelPicker({
  profile,
  models,
  configOptions,
  baselineProfile,
  labelCls,
  gapCls,
  disabled,
  onChange,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  configOptions: SelectConfigOption[];
  baselineProfile?: ProfileFormData;
  labelCls?: string;
  gapCls: string;
  disabled?: boolean;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  const [optedIn, setOptedIn] = useState(false);
  const fallbackEnabled = Boolean(profile.fallback_model) || optedIn;
  // The fallback picker must not inherit the START model's current value or
  // config options: an enabled-but-empty fallback shows its placeholder, and
  // selecting a model through the config-option path still lands in
  // fallback_model. Rebuild the options with the fallback's own current value.
  const fallbackConfigOptions = configOptions.map((option) => ({
    ...option,
    currentValue: isModelConfigOption(option)
      ? (profile.fallback_model ?? "")
      : option.currentValue,
  }));
  const fallbackModelOptionId = configOptions.find(isModelConfigOption)?.id;
  return (
    <div
      data-testid="profile-fallback-model-field"
      className={gapCls}
      data-settings-dirty={profileFallbackModelIsDirty(profile, baselineProfile)}
      data-settings-dirty-level="container"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-1">
          <SettingsFieldLabel className={labelCls}>
            {t("settings:agentFallback")}
          </SettingsFieldLabel>
          <FallbackOptionHelp kind="explicit" />
        </div>
        <Switch
          checked={fallbackEnabled}
          disabled={disabled}
          onCheckedChange={(checked) => {
            setOptedIn(checked);
            if (!checked) onChange({ fallback_model: "" });
          }}
          aria-label={t("settings:agentFallback")}
        />
      </div>
      {fallbackEnabled && (
        <div className="min-w-0">
          <ModelPicker
            profile={{ ...profile, model: profile.fallback_model ?? "" }}
            models={models}
            currentModelId={profile.fallback_model || undefined}
            configOptions={fallbackConfigOptions}
            onChange={(patch) => {
              if (patch.model !== undefined) {
                onChange({ fallback_model: patch.model });
                return;
              }
              if (patch.config_options && fallbackModelOptionId) {
                const selected = patch.config_options[fallbackModelOptionId];
                if (selected !== undefined) onChange({ fallback_model: selected });
              }
            }}
            ariaLabel={t("settings:agentFallbackAria")}
            placeholder={t("settings:agentFallbackPlaceholder")}
            goneModelLabel={t("settings:fallbackModelUnavailable")}
            disabled={disabled}
          />
        </div>
      )}
      <SettingsFieldDescription>
        {disabled ? t("settings:agentFallbackDisabledHelper") : t("settings:agentFallbackHelper")}
      </SettingsFieldDescription>
    </div>
  );
}

// Auto-fallback (legacy behavior) is mutually exclusive with a per-profile
// fallback model: the toggle switches between the two. Selecting a fallback
// model is only meaningful when auto-fallback is off, because auto-fallback
// already allows the provider to switch to any advertised model — the
// configured fallback would be redundant. The strict mode (both off) is the
// only automatic model switch allowed when auto-fallback is off.
export function ModelFallbackSection({
  profile,
  models,
  configOptions,
  baselineProfile,
  labelCls,
  gapCls,
  onChange,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  configOptions: SelectConfigOption[];
  baselineProfile?: ProfileFormData;
  labelCls?: string;
  gapCls: string;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  const autoFallback = profile.auto_fallback ?? false;
  const isDirty =
    profileAutoFallbackIsDirty(profile, baselineProfile) ||
    profileFallbackModelIsDirty(profile, baselineProfile);

  return (
    <ModelFallbackSettingsShell
      autoFallback={autoFallback}
      fallbackModel={profile.fallback_model ?? ""}
      isDirty={isDirty}
      automaticOption={
        <div
          data-testid="profile-auto-fallback-field"
          className={`min-w-0 ${gapCls}`}
          data-settings-dirty={profileAutoFallbackIsDirty(profile, baselineProfile)}
          data-settings-dirty-level="container"
        >
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-1">
              <SettingsFieldLabel className={labelCls}>
                {t("settings:autoFallback")}
              </SettingsFieldLabel>
              <FallbackOptionHelp kind="automatic" />
            </div>
            <Switch
              checked={autoFallback}
              onCheckedChange={(checked) => onChange({ auto_fallback: checked })}
              aria-label={t("settings:autoFallback")}
            />
          </div>
          <SettingsFieldDescription>{t("settings:autoFallbackHelper")}</SettingsFieldDescription>
        </div>
      }
      explicitOption={
        <FallbackModelPicker
          profile={profile}
          models={models}
          configOptions={configOptions}
          baselineProfile={baselineProfile}
          labelCls={labelCls}
          gapCls={gapCls}
          disabled={autoFallback}
          onChange={onChange}
        />
      }
    />
  );
}
