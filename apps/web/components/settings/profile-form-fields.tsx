"use client";

import { useId } from "react";
import { IconAlertCircle, IconAlertTriangle, IconRefresh } from "@tabler/icons-react";
import { NoAuthPanel, ProbingPanel } from "@/components/settings/profile-status-panels";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ModeCombobox } from "@/components/settings/mode-combobox";
import {
  configOptionToModelOptions,
  isModelConfigOption,
  ModelConfigSelector,
  type SelectConfigOption,
  usableConfigOptions,
} from "@/components/model-config-selector";
import { useAgentCapabilities } from "@/hooks/domains/settings/use-dynamic-models";
import {
  PERMISSION_APPLY_AGENTCTL_AUTO_APPROVE,
  PERMISSION_KEYS,
  readPermissionValue,
  type PermissionKey,
} from "@/lib/agent-permissions";
import { CLIFlagsField } from "@/components/settings/cli-flags-field";
import { CommandPrefixField } from "@/components/settings/command-prefix-field";
import {
  CommandsButton,
  findActiveMode,
  profileModeIsDirty,
  profileModelIsDirty,
} from "@/components/settings/profile-capability-helpers";
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
        <Label htmlFor={switchId} className={`flex items-center gap-1.5 ${labelCls ?? ""}`}>
          {isDanger && <IconAlertTriangle className="size-4 shrink-0 text-destructive" />}
          {setting.label}
        </Label>
        <p
          className={
            compact
              ? "text-[10px] text-muted-foreground leading-tight"
              : "text-xs text-muted-foreground"
          }
        >
          {setting.description}
        </p>
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
              <Label className="text-xs">{passthroughConfig.label}</Label>
              <p className="text-[10px] text-muted-foreground leading-tight">
                {passthroughConfig.description}
              </p>
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
            <Label>{passthroughConfig.label}</Label>
            <p className="text-xs text-muted-foreground">{passthroughConfig.description}</p>
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

function capabilityStatusMessage(status: ModelConfig["status"]): string | null {
  switch (status) {
    case "probing":
      return "Checking agent capabilities…";
    case "auth_required":
      return "Authentication required. Run the agent CLI in your terminal to authenticate, then refresh.";
    case "not_installed":
      return "Agent CLI not installed.";
    case "failed":
      return "Probe failed. Check agent logs for details.";
    default:
      return null;
  }
}

function CapabilityStatusMessage({ status }: { status: ModelConfig["status"] }) {
  const msg = capabilityStatusMessage(status);
  if (!msg) return null;
  return (
    <p
      data-testid="profile-capability-status"
      data-status={status}
      className="text-xs text-muted-foreground"
    >
      {msg}
    </p>
  );
}

function RefreshCapabilitiesButton({
  onRefresh,
  isLoading,
  error,
}: {
  onRefresh: () => Promise<void>;
  isLoading: boolean;
  error: string | null;
}) {
  return (
    <div className="flex items-center gap-2">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            onClick={onRefresh}
            disabled={isLoading}
            className="cursor-pointer"
            data-testid="profile-refresh-capabilities"
          >
            <IconRefresh className={`h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>Refresh agent capabilities (models + modes)</p>
        </TooltipContent>
      </Tooltip>
      {error && (
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="flex items-center">
              <IconAlertCircle className="h-4 w-4 text-amber-500" />
            </div>
          </TooltipTrigger>
          <TooltipContent>
            <p className="max-w-xs">Failed to refresh: {error}</p>
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function ModelPicker({
  profile,
  models,
  currentModelId,
  configOptions,
  onChange,
}: {
  profile: ProfileFormData;
  models: ModelEntry[];
  currentModelId: string | undefined;
  configOptions: SelectConfigOption[];
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const modelConfig = configOptions.find(isModelConfigOption);
  const modelOptions = modelConfig
    ? configOptionToModelOptions(modelConfig)
    : models.map((model) => ({
        id: model.id,
        name: model.name,
        description: model.description || (model.id !== model.name ? model.id : undefined),
        usageMultiplier:
          typeof model.meta?.copilotUsage === "string" ? model.meta.copilotUsage : undefined,
      }));
  const currentModel = profile.model || modelConfig?.currentValue || currentModelId || null;
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
      placeholder="Select a model..."
      ariaLabel="Profile start model settings"
    />
  );
}

function ModePicker({
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

function modelConfigOptions(modelConfig: ModelConfig): SelectConfigOption[] {
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
  agentName: string;
  baselineProfile?: ProfileFormData;
};

function CapabilitiesRow({
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
  agentName,
  baselineProfile,
}: CapabilitiesRowProps) {
  const hasModes = modes.length > 0;
  const configOptions = modelConfigOptions(modelConfig);
  const activeMode = findActiveMode(modes, profile.mode, currentModeId);
  const labelCls = isCompact ? "text-xs text-muted-foreground" : undefined;
  const gapCls = isCompact ? "space-y-1.5" : "space-y-2";

  if (isLoading && models.length === 0) {
    return (
      <div className={gapCls}>
        <Label className={labelCls}>Start model</Label>
        <Skeleton className="h-7 w-full" />
      </div>
    );
  }

  if (status === "probing") {
    return <ProbingPanel />;
  }
  if (status === "auth_required" || status === "not_installed") {
    return (
      <NoAuthPanel
        agentName={agentName}
        status={status}
        isLoading={isLoading}
        onRefresh={onRefresh}
        error={error}
        rawError={modelConfig.error ?? null}
      />
    );
  }

  return (
    <div className={gapCls}>
      <div className="flex items-end gap-2">
        <div
          className={`flex-1 min-w-0 ${gapCls}`}
          data-settings-dirty={profileModelIsDirty(profile, baselineProfile)}
          data-settings-dirty-level="container"
        >
          <Label className={labelCls}>Start model</Label>
          <ModelPicker
            profile={profile}
            models={models}
            currentModelId={currentModelId}
            configOptions={configOptions}
            onChange={onChange}
          />
        </div>
        {hasModes && (
          <div
            data-testid="profile-mode-field"
            className={`flex-1 min-w-0 ${gapCls}`}
            data-settings-dirty={profileModeIsDirty(profile, baselineProfile)}
            data-settings-dirty-level="container"
          >
            <Label className={labelCls}>Start mode</Label>
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
      {activeMode?.description && (
        <p className="text-xs text-muted-foreground">{activeMode.description}</p>
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
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="flex-1 space-y-2">
        <Label>Profile name</Label>
        <Input
          data-testid="profile-name-input"
          value={profile.name}
          onChange={(event) => onChange({ name: event.target.value })}
          placeholder="Default profile"
          data-settings-dirty={baselineName !== undefined && profile.name !== baselineName}
        />
      </div>
      {canRemove && onRemove && (
        <Button size="sm" variant="ghost" className="cursor-pointer" onClick={onRemove}>
          Remove
        </Button>
      )}
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
}: ProfileFormFieldsProps) {
  const isCompact = variant === "compact";
  const caps = useAgentCapabilities(agentName, modelConfig);

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
        onRefresh={caps.refresh}
        error={caps.error}
        modelConfig={modelConfig}
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

      {!profile.cli_passthrough && (
        <CommandPrefixField
          profile={profile}
          baselineProfile={baselineProfile}
          onChange={onChange}
        />
      )}
    </div>
  );
}
