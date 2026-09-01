"use client";

import { useEffect, useId } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import type { SelectConfigOption } from "@/components/model-config-selector";
import { NoAuthPanel, ProbingPanel } from "@/components/settings/profile-status-panels";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import { useProfileModelCapabilities } from "@/hooks/domains/settings/use-profile-model-capabilities";
import {
  PERMISSION_APPLY_AGENTCTL_AUTO_APPROVE,
  PERMISSION_KEYS,
  readPermissionValue,
  type PermissionKey,
} from "@/lib/agent-permissions";
import { CLIFlagsField } from "@/components/settings/cli-flags-field";
import { ProfileAdvancedOptions } from "@/components/settings/profile-advanced-options";
import { ModelConfigResolutionStatus } from "@/components/settings/model-config-resolution-status";
import {
  CapabilityStatusMessage,
  RefreshCapabilitiesButton,
} from "@/components/settings/profile-capability-status";
import {
  CommandsButton,
  findActiveMode,
  profileModeIsDirty,
  profileModelIsDirty,
} from "@/components/settings/profile-capability-helpers";
import { modelConfigOptions } from "@/components/settings/profile-model-config";
import {
  ModelFallbackSection,
  ModelPicker,
  ModePicker,
} from "@/components/settings/profile-model-fields";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import type {
  CLIFlag,
  CommandEntry,
  ModelConfig,
  ModeEntry,
  ModelEntry,
  PermissionSetting,
  PassthroughConfig,
} from "@/lib/types/http";

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

export type ProfileFormFieldsProps = {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  agentName: string;
  onRemove?: () => void;
  canRemove?: boolean;
  variant?: "default" | "compact";
  hideNameField?: boolean;
  lockPassthrough?: boolean;
  onModelConfigResolutionPendingChange?: (pending: boolean) => void;
  /**
   * When true, the custom-flag list + Add form on CLIFlagsField is
   * hidden. Curated predefined toggles still render. Used by the
   * onboarding flow to keep the first-run UI narrow.
   */
  hideCustomCLIFlags?: boolean;
};

type PermissionToggleProps = {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  variant: "default" | "compact";
  lockPassthrough?: boolean;
};

function permissionToggleWrapperClass(isDanger: boolean, compact: boolean): string {
  if (isDanger) {
    return "flex items-center justify-between gap-3 rounded-md border border-destructive/40 bg-destructive/5 p-3";
  }
  if (compact) {
    return "flex items-center justify-between gap-2";
  }
  return "flex items-center justify-between rounded-md border p-3";
}

function PermissionToggleRow({
  settingKey,
  setting,
  checked,
  onCheckedChange,
  compact,
  isDirty,
}: {
  settingKey: string;
  setting: PermissionSetting;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  compact: boolean;
  isDirty: boolean;
}) {
  const isDanger = setting.apply_method === PERMISSION_APPLY_AGENTCTL_AUTO_APPROVE;
  const switchSize = compact ? ("sm" as const) : ("default" as const);
  const labelCls = compact ? "text-xs" : undefined;
  const wrapperCls = permissionToggleWrapperClass(isDanger, compact);
  const instanceId = useId();
  const switchId = `${instanceId}-permission-toggle-${settingKey}`;

  return (
    <div
      key={settingKey}
      className={wrapperCls}
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
      data-testid={isDanger ? "permission-auto-approve-danger" : `permission-toggle-${settingKey}`}
    >
      <div className={`flex-1 min-w-0 ${compact && !isDanger ? "space-y-0.5" : "space-y-1"}`}>
        <SettingsFieldLabel
          htmlFor={switchId}
          className={`flex items-center gap-1.5 ${labelCls ?? ""}`}
        >
          {isDanger && <IconAlertTriangle className="size-4 shrink-0 text-destructive" />}
          {setting.label}
        </SettingsFieldLabel>
        <SettingsFieldDescription>{setting.description}</SettingsFieldDescription>
      </div>
      <Switch id={switchId} size={switchSize} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}

function PermissionToggles({
  profile,
  onChange,
  permissionSettings,
  passthroughConfig,
  variant,
  lockPassthrough,
  baselineProfile,
}: PermissionToggleProps) {
  const isCompact = variant === "compact";
  const switchSize = isCompact ? ("sm" as const) : ("default" as const);

  if (isCompact) {
    return (
      <>
        {PERMISSION_KEYS.map((key) => {
          const setting = permissionSettings[key];
          if (!setting?.supported) return null;
          if (setting.apply_method === "cli_flag") return null;
          const checked = readPermissionValue(profile, key, permissionSettings);
          return (
            <PermissionToggleRow
              key={key}
              settingKey={key}
              setting={setting}
              checked={checked}
              onCheckedChange={(checked) => onChange({ [key]: checked })}
              compact
              isDirty={
                Boolean(baselineProfile) &&
                checked !== readPermissionValue(baselineProfile!, key, permissionSettings)
              }
            />
          );
        })}
        {passthroughConfig?.supported && (
          <div className="flex items-center justify-between gap-2">
            <div className="space-y-0.5">
              <SettingsFieldLabel className="text-xs">{passthroughConfig.label}</SettingsFieldLabel>
              <SettingsFieldDescription>{passthroughConfig.description}</SettingsFieldDescription>
            </div>
            <Switch
              size={switchSize}
              checked={profile.cli_passthrough}
              onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
              disabled={lockPassthrough}
              data-settings-dirty={
                Boolean(baselineProfile) &&
                profile.cli_passthrough !== baselineProfile?.cli_passthrough
              }
            />
          </div>
        )}
      </>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2">
      {PERMISSION_KEYS.map((key) => {
        const setting = permissionSettings[key];
        if (!setting?.supported) return null;
        if (setting.apply_method === "cli_flag") return null;
        return (
          <PermissionToggleRow
            key={key}
            settingKey={key}
            setting={setting}
            checked={readPermissionValue(profile, key, permissionSettings)}
            onCheckedChange={(checked) => onChange({ [key]: checked })}
            compact={false}
            isDirty={
              Boolean(baselineProfile) &&
              readPermissionValue(profile, key, permissionSettings) !==
                readPermissionValue(baselineProfile!, key, permissionSettings)
            }
          />
        );
      })}
      {passthroughConfig?.supported && (
        <div className="flex items-center justify-between rounded-md border p-3">
          <div className="space-y-1">
            <SettingsFieldLabel>{passthroughConfig.label}</SettingsFieldLabel>
            <SettingsFieldDescription>{passthroughConfig.description}</SettingsFieldDescription>
          </div>
          <Switch
            checked={profile.cli_passthrough}
            onCheckedChange={(checked) => onChange({ cli_passthrough: checked })}
            disabled={lockPassthrough}
            data-settings-dirty={
              Boolean(baselineProfile) &&
              profile.cli_passthrough !== baselineProfile?.cli_passthrough
            }
          />
        </div>
      )}
    </div>
  );
}

type CapabilitiesRowProps = {
  profile: ProfileFormData;
  models: ModelEntry[];
  modes: ModeEntry[];
  commands: CommandEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  status: ModelConfig["status"];
  onChange: (patch: Partial<ProfileFormData>) => void;
  isCompact: boolean;
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  modelConfig: ModelConfig;
  configOptions: SelectConfigOption[];
  configStatus: ModelConfig["status"];
  configError: string | null;
  configIsLoading: boolean;
  onRetryConfig: () => Promise<void>;
  agentName: string;
  baselineProfile?: ProfileFormData;
};

function CapabilitiesRow(props: CapabilitiesRowProps) {
  const { t } = useTranslation();
  const gapCls = props.isCompact ? "space-y-1.5" : "space-y-2";

  if (props.isLoading && props.models.length === 0) {
    return (
      <div className={gapCls}>
        <SettingsFieldLabel className={props.isCompact ? "text-xs" : undefined}>
          {t("agents:startModel")}
        </SettingsFieldLabel>
        <Skeleton className="h-7 w-full" />
      </div>
    );
  }

  if (props.status === "probing") {
    return <ProbingPanel />;
  }
  if (props.status === "auth_required" || props.status === "not_installed") {
    return (
      <NoAuthPanel
        agentName={props.agentName}
        status={props.status}
        isLoading={props.isLoading}
        onRefresh={props.onRefresh}
        error={props.error}
        rawError={props.modelConfig.error ?? null}
      />
    );
  }

  return <CapabilitiesRowContent {...props} />;
}

function CapabilitiesRowContent({
  profile,
  models,
  modes,
  commands,
  currentModelId,
  currentModeId,
  status,
  onChange,
  isCompact,
  isLoading,
  onRefresh,
  error,
  modelConfig,
  configOptions,
  configStatus,
  configError,
  configIsLoading,
  onRetryConfig,
  baselineProfile,
}: CapabilitiesRowProps) {
  const { t } = useTranslation();
  const hasModes = modes.length > 0;
  const activeMode = findActiveMode(modes, profile.mode, currentModeId);
  const labelCls = isCompact ? "text-xs" : undefined;
  const gapCls = isCompact ? "space-y-1.5" : "space-y-2";

  return (
    <div className={gapCls}>
      <div className="flex items-end gap-2" data-testid="profile-capabilities-model-row">
        <div
          className={`${hasModes ? "flex-1" : "w-full md:max-w-xl"} min-w-0 ${gapCls}`}
          data-settings-dirty={profileModelIsDirty(profile, baselineProfile)}
          data-settings-dirty-level="container"
        >
          <SettingsFieldLabel className={labelCls}>{t("agents:startModel")}</SettingsFieldLabel>
          <ModelPicker
            profile={profile}
            models={models}
            currentModelId={currentModelId}
            configOptions={configOptions}
            onChange={onChange}
            ariaLabel={t("settings:startModelAria")}
            goneModelLabel={t("settings:startModelUnavailable")}
            configOptionsLoading={configIsLoading}
            keepOpenOnModelChange={modelConfig.supports_dynamic_models}
          />
        </div>
        {hasModes && (
          <div
            data-testid="profile-mode-field"
            className={`flex-1 min-w-0 ${gapCls}`}
            data-settings-dirty={profileModeIsDirty(profile, baselineProfile)}
            data-settings-dirty-level="container"
          >
            <SettingsFieldLabel className={labelCls}>{t("agents:startMode")}</SettingsFieldLabel>
            <ModePicker
              profile={profile}
              modes={modes}
              currentModeId={currentModeId}
              onChange={onChange}
            />
          </div>
        )}
        <RefreshCapabilitiesButton onRefresh={onRefresh} isLoading={isLoading} error={error} />
      </div>
      <ModelConfigResolutionStatus
        status={configStatus}
        error={configError}
        isLoading={configIsLoading}
        onRetry={onRetryConfig}
      />
      {activeMode?.description && (
        <SettingsFieldDescription>{activeMode.description}</SettingsFieldDescription>
      )}
      {commands.length > 0 && <CommandsButton commands={commands} />}
      <CapabilityStatusMessage status={status} />
    </div>
  );
}

function NameField({
  profile,
  onChange,
  canRemove,
  onRemove,
  baselineName,
}: {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  canRemove?: boolean;
  onRemove?: () => void;
  baselineName?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-4">
      <ProfileNameField
        value={profile.name}
        onChange={(value) => onChange({ name: value })}
        dirty={baselineName !== undefined && profile.name !== baselineName}
      />
      {canRemove && onRemove && (
        <Button size="sm" variant="ghost" className="cursor-pointer" onClick={onRemove}>
          {t("agents:remove")}
        </Button>
      )}
    </div>
  );
}

export type ProfileNameFieldProps = {
  value: string;
  onChange: (value: string) => void;
  dirty?: boolean;
  id?: string;
  testId?: string;
};

export function ProfileNameField({
  value,
  onChange,
  dirty = false,
  id,
  testId = "profile-name-input",
}: ProfileNameFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="flex-1 space-y-2">
      <SettingsFieldLabel htmlFor={id}>{t("agents:profileName")}</SettingsFieldLabel>
      <Input
        id={id}
        data-testid={testId}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t("agents:defaultProfile")}
        data-settings-dirty={dirty}
      />
    </div>
  );
}

export function ProfileFormFields({
  profile,
  baselineProfile,
  onChange,
  modelConfig,
  permissionSettings,
  passthroughConfig,
  agentName,
  onRemove,
  canRemove = false,
  variant = "default",
  hideNameField = false,
  lockPassthrough = false,
  hideCustomCLIFlags = false,
  onModelConfigResolutionPendingChange,
}: ProfileFormFieldsProps) {
  const isCompact = variant === "compact";
  const {
    capabilities: caps,
    configOptions: resolvedConfigOptions,
    configStatus,
    configError,
    configIsLoading,
    isConfigResolutionPending,
    refreshModelConfig,
    refresh,
  } = useProfileModelCapabilities(agentName, profile, modelConfig, onChange);
  const configOptions = modelConfigOptions(
    resolvedConfigOptions ? { ...modelConfig, config_options: resolvedConfigOptions } : modelConfig,
  );

  useEffect(() => {
    onModelConfigResolutionPendingChange?.(isConfigResolutionPending);
  }, [isConfigResolutionPending, onModelConfigResolutionPendingChange]);

  return (
    <div className={isCompact ? "space-y-3" : "space-y-4"}>
      {!hideNameField && (
        <NameField
          profile={profile}
          onChange={onChange}
          canRemove={canRemove}
          onRemove={onRemove}
          baselineName={baselineProfile?.name}
        />
      )}

      <CapabilitiesRow
        profile={profile}
        models={caps.models}
        modes={caps.modes}
        commands={caps.commands}
        currentModelId={caps.currentModelId}
        currentModeId={caps.currentModeId}
        status={caps.status}
        agentName={agentName}
        onChange={onChange}
        isCompact={isCompact}
        isLoading={caps.isLoading}
        onRefresh={refresh}
        error={caps.error}
        modelConfig={modelConfig}
        configOptions={configOptions}
        configStatus={configStatus}
        configError={configError}
        configIsLoading={configIsLoading}
        onRetryConfig={refreshModelConfig}
        baselineProfile={baselineProfile}
      />

      <PermissionToggles
        profile={profile}
        onChange={onChange}
        permissionSettings={permissionSettings}
        passthroughConfig={passthroughConfig}
        variant={variant}
        lockPassthrough={lockPassthrough}
        baselineProfile={baselineProfile}
      />

      <ProfileFormFooter
        profile={profile}
        baselineProfile={baselineProfile}
        onChange={onChange}
        permissionSettings={permissionSettings}
        variant={variant}
        hideCustomCLIFlags={hideCustomCLIFlags}
        models={caps.models}
        configOptions={configOptions}
        isCompact={isCompact}
      />
    </div>
  );
}

function ProfileFormFooter({
  profile,
  baselineProfile,
  onChange,
  permissionSettings,
  variant,
  hideCustomCLIFlags,
  models,
  configOptions,
  isCompact,
}: {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  permissionSettings: Record<string, PermissionSetting>;
  variant: "default" | "compact";
  hideCustomCLIFlags: boolean;
  models: ModelEntry[];
  configOptions: SelectConfigOption[];
  isCompact: boolean;
}) {
  return (
    <>
      <div
        data-settings-dirty={
          Boolean(baselineProfile) &&
          JSON.stringify(profile.cli_flags) !== JSON.stringify(baselineProfile?.cli_flags)
        }
        data-settings-dirty-level="container"
      >
        <CLIFlagsField
          flags={profile.cli_flags}
          onChange={(next) => onChange({ cli_flags: next })}
          permissionSettings={permissionSettings}
          variant={variant}
          hideCustomFlags={hideCustomCLIFlags}
        />
      </div>

      <div className="space-y-1" data-testid="profile-disclosure-stack">
        <ModelFallbackSection
          profile={profile}
          models={models}
          configOptions={configOptions}
          baselineProfile={baselineProfile}
          labelCls={isCompact ? "text-xs" : undefined}
          gapCls={isCompact ? "space-y-1.5" : "space-y-2"}
          onChange={onChange}
        />

        {!profile.cli_passthrough && (
          <ProfileAdvancedOptions
            profile={profile}
            baselineProfile={baselineProfile}
            onChange={onChange}
          />
        )}
      </div>
    </>
  );
}
