"use client";

import { useTranslation } from "react-i18next";
import type { SelectConfigOption } from "@/components/model-config-selector";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import type { PermissionKey } from "@/lib/agent-permissions";
import { PermissionToggles } from "@/components/settings/profile-permission-toggles";
import { CLIFlagsField } from "@/components/settings/cli-flags-field";
import { CursorPluginsMCPPreference } from "@/components/settings/cursor-plugins-mcp-preference";
import type { MCPSelectionMode } from "@/lib/types/agent-profile";
import { CursorMCPAuthPreference } from "@/components/settings/cursor-mcp-auth-preference";
import { ProfileAdvancedOptions } from "@/components/settings/profile-advanced-options";
import { CopilotPluginProfileSelection } from "@/components/settings/copilot-plugin-profile-selection";
import { useProfileFormCapabilities } from "@/components/settings/profile-capability-helpers";
import { ProfileCapabilitiesSection } from "@/components/settings/profile-capabilities-row";
import { ModelFallbackSection } from "@/components/settings/profile-model-fields";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import type {
  CLIFlag,
  ModelConfig,
  ModelEntry,
  PermissionSetting,
  PassthroughConfig,
  ProfileLaunchSettingsRequest,
} from "@/lib/types/http";

export type ProfileFormData = {
  name: string;
  model: string;
  /** Optional single fallback model applied when `model` is unavailable. */
  fallback_model?: string;
  /** Legacy automatic-fallback opt-in; hides the fallback_model field. */
  auto_fallback?: boolean;
  require_exact_model?: boolean;
  mode: string;
  config_options?: Record<string, string>;
  cli_passthrough: boolean;
  cli_flags: CLIFlag[];
  env_vars?: { key: string; value?: string; secret_id?: string }[];
  command_prefix?: string;
  provider_kind?: string;
  cursor_mcp_auth_enabled?: boolean;
  cursor_plugins_mcp_enabled?: boolean;
  mcp_selection_mode?: MCPSelectionMode;
  mcp_selected_servers?: string[];
  copilot_selected_plugins?: string[];
} & Record<PermissionKey, boolean>;

export type ProfileFormFieldsProps = {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  agentName: string;
  agentId?: string;
  copilotDiscoveryDisabled?: boolean;
  cursorMcpAuthSupported?: boolean;
  onRemove?: () => void;
  canRemove?: boolean;
  variant?: "default" | "compact";
  hideNameField?: boolean;
  lockPassthrough?: boolean;
  onModelConfigResolutionPendingChange?: (pending: boolean) => void;
  capabilityProfileId?: string;
  /**
   * When true, the custom-flag list + Add form on CLIFlagsField is
   * hidden. Curated predefined toggles still render. Used by the
   * onboarding flow to keep the first-run UI narrow.
   */
  hideCustomCLIFlags?: boolean;
};

function NameField({
  profile,
  onChange,
  canRemove,
  onRemove,
  baselineName,
  visible = true,
}: {
  profile: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  canRemove?: boolean;
  onRemove?: () => void;
  baselineName?: string;
  visible?: boolean;
}) {
  const { t } = useTranslation();
  if (!visible) return null;
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

function CursorProfilePreferences({
  supported,
  profile,
  baselineProfile,
  onChange,
}: {
  supported: boolean;
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  if (!supported) return null;
  return (
    <>
      <CursorMCPAuthPreference
        enabled={profile.cursor_mcp_auth_enabled ?? true}
        savedEnabled={
          baselineProfile === undefined
            ? undefined
            : (baselineProfile.cursor_mcp_auth_enabled ?? true)
        }
        onChange={(enabled) => onChange({ cursor_mcp_auth_enabled: enabled })}
      />
      <CursorPluginsMCPPreference
        enabled={profile.cursor_plugins_mcp_enabled ?? true}
        savedEnabled={
          baselineProfile === undefined
            ? undefined
            : (baselineProfile.cursor_plugins_mcp_enabled ?? true)
        }
        onChange={(enabled) => onChange({ cursor_plugins_mcp_enabled: enabled })}
      />
    </>
  );
}

function savedProfileLaunchSettings(
  baselineProfile?: ProfileFormData,
): ProfileLaunchSettingsRequest | undefined {
  if (!baselineProfile) return undefined;
  return {
    env_vars: baselineProfile.env_vars ?? [],
    cli_flags: baselineProfile.cli_flags ?? [],
    command_prefix: baselineProfile.command_prefix ?? "",
    ...(baselineProfile.copilot_selected_plugins !== undefined
      ? { copilot_selected_plugins: [...baselineProfile.copilot_selected_plugins] }
      : {}),
  };
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
  agentId,
  copilotDiscoveryDisabled = false,
  cursorMcpAuthSupported = false,
  onRemove,
  canRemove = false,
  variant = "default",
  hideNameField = false,
  lockPassthrough = false,
  hideCustomCLIFlags = false,
  onModelConfigResolutionPendingChange,
  capabilityProfileId,
}: ProfileFormFieldsProps) {
  const isCompact = variant === "compact";
  const capabilityResult = useProfileFormCapabilities(agentName, profile, modelConfig, onChange, {
    profileId: capabilityProfileId,
    onPendingChange: onModelConfigResolutionPendingChange,
    savedLaunchSettings: savedProfileLaunchSettings(baselineProfile),
  });

  return (
    <div className={isCompact ? "space-y-3" : "space-y-4"}>
      <NameField
        profile={profile}
        onChange={onChange}
        canRemove={canRemove}
        onRemove={onRemove}
        baselineName={baselineProfile?.name}
        visible={!hideNameField}
      />

      <ProfileCapabilitiesSection
        profile={profile}
        baselineProfile={baselineProfile}
        onChange={onChange}
        modelConfig={modelConfig}
        agentName={agentName}
        isCompact={isCompact}
        result={capabilityResult}
      />

      <PermissionToggles
        agentName={agentName}
        profile={profile}
        onChange={onChange}
        permissionSettings={permissionSettings}
        passthroughConfig={passthroughConfig}
        variant={variant}
        lockPassthrough={lockPassthrough}
        baselineProfile={baselineProfile}
      />

      <CursorProfilePreferences
        supported={cursorMcpAuthSupported}
        profile={profile}
        baselineProfile={baselineProfile}
        onChange={onChange}
      />

      <CopilotProfilePlugins
        agentName={agentName}
        agentId={agentId}
        profile={profile}
        baselineProfile={baselineProfile}
        capabilityProfileId={capabilityProfileId}
        discoveryDisabled={copilotDiscoveryDisabled}
        onChange={onChange}
      />

      <ProfileFormFooter
        profile={profile}
        baselineProfile={baselineProfile}
        onChange={onChange}
        permissionSettings={permissionSettings}
        variant={variant}
        hideCustomCLIFlags={hideCustomCLIFlags}
        models={capabilityResult.capabilities.models}
        configOptions={capabilityResult.configOptions}
        isCompact={isCompact}
        disableUnverifiedModels={capabilityResult.discoveryState !== "ready"}
      />
    </div>
  );
}

function CopilotProfilePlugins({
  agentName,
  agentId,
  profile,
  baselineProfile,
  capabilityProfileId,
  discoveryDisabled,
  onChange,
}: {
  agentName: string;
  agentId?: string;
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  capabilityProfileId?: string;
  discoveryDisabled: boolean;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  if (discoveryDisabled || agentName !== "copilot-acp" || !agentId) return null;
  return (
    <CopilotPluginProfileSelection
      agentId={agentId}
      profileId={capabilityProfileId ?? ""}
      launchSettings={{
        env_vars: profile.env_vars ?? [],
        cli_flags: profile.cli_flags ?? [],
        command_prefix: profile.command_prefix ?? "",
      }}
      selectedPluginIds={profile.copilot_selected_plugins ?? []}
      savedSelectedPluginIds={baselineProfile?.copilot_selected_plugins}
      onSelectedPluginsChange={(copilot_selected_plugins) => onChange({ copilot_selected_plugins })}
    />
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
  disableUnverifiedModels,
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
  disableUnverifiedModels: boolean;
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
          disabled={disableUnverifiedModels}
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
