import { useTranslation } from "react-i18next";
import type { SelectConfigOption } from "@/components/model-config-selector";
import { ModelConfigResolutionStatus } from "@/components/settings/model-config-resolution-status";
import {
  CapabilityStatusMessage,
  RefreshCapabilitiesButton,
} from "@/components/settings/profile-capability-status";
import { NoAuthPanel } from "@/components/settings/profile-status-panels";
import {
  CommandsButton,
  findActiveMode,
  profileModeIsDirty,
  profileModelIsDirty,
} from "@/components/settings/profile-capability-helpers";
import { ModelPicker, ModePicker } from "@/components/settings/profile-model-fields";
import type { ProfileDiscoveryStatus } from "@/hooks/domains/settings/use-profile-model-capabilities";
import type { CommandEntry, ModelConfig, ModeEntry, ModelEntry } from "@/lib/types/http";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import type { ProfileFormData } from "./profile-form-fields";
import type { useProfileFormCapabilities } from "./profile-capability-helpers";

type CapabilitiesRowProps = {
  profile: ProfileFormData;
  models: ModelEntry[];
  modes: ModeEntry[];
  commands: CommandEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  status: ModelConfig["status"];
  discoveryState: ProfileDiscoveryStatus;
  disableUnverifiedModels: boolean;
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
  if (props.profile.provider_kind === "openai_compatible") {
    return <CapabilitiesRowContent {...props} discoveryState="ready" isLoading={false} />;
  }
  return <CapabilitiesRowContent {...props} />;
}

function StatusOrAuthMessage({
  agentName,
  status,
  isLoading,
  onRefresh,
  error,
  discoveryState,
}: {
  agentName: string;
  status: ModelConfig["status"];
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  discoveryState: ProfileDiscoveryStatus;
}) {
  if (status === "auth_required" || status === "not_installed") {
    return (
      <NoAuthPanel
        agentName={agentName}
        status={status}
        isLoading={isLoading}
        onRefresh={onRefresh}
        error={error}
        rawError={null}
      />
    );
  }
  return <CapabilityStatusMessage status={discoveryState} />;
}

type ModelAndModePickersProps = {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  models: ModelEntry[];
  modes: ModeEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  configOptions: SelectConfigOption[];
  onChange: (patch: Partial<ProfileFormData>) => void;
  disableUnverifiedModels: boolean;
  configIsLoading: boolean;
  supportsDynamicModels?: boolean;
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  showRefresh: boolean;
  labelCls?: string;
  gapCls: string;
};

function ModelAndModePickers({
  profile,
  baselineProfile,
  models,
  modes,
  currentModelId,
  currentModeId,
  configOptions,
  onChange,
  disableUnverifiedModels,
  configIsLoading,
  supportsDynamicModels,
  isLoading,
  onRefresh,
  error,
  showRefresh,
  labelCls,
  gapCls,
}: ModelAndModePickersProps) {
  const { t } = useTranslation();
  const hasModes = modes.length > 0;

  return (
    <div
      className={
        hasModes
          ? "grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]"
          : "grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2 md:max-w-xl"
      }
      data-testid="profile-capabilities-model-row"
    >
      <div className="min-w-0">
        <div
          className="flex-1 min-w-0 space-y-1.5 sm:space-y-2"
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
            disabled={disableUnverifiedModels}
            configOptionsLoading={configIsLoading}
            keepOpenOnModelChange={supportsDynamicModels}
          />
        </div>
      </div>
      {hasModes && (
        <div
          data-testid="profile-mode-field"
          className={`col-span-2 row-start-2 min-w-0 md:col-span-1 md:col-start-2 md:row-start-1 ${gapCls}`}
          data-settings-dirty={profileModeIsDirty(profile, baselineProfile)}
          data-settings-dirty-level="container"
        >
          <SettingsFieldLabel className={labelCls}>{t("agents:startMode")}</SettingsFieldLabel>
          <ModePicker
            profile={profile}
            modes={modes}
            currentModeId={currentModeId}
            onChange={onChange}
            disabled={disableUnverifiedModels}
          />
        </div>
      )}
      {showRefresh && (
        <div className={`col-start-2 row-start-1 shrink-0 ${hasModes ? "md:col-start-3" : ""}`}>
          <RefreshCapabilitiesButton onRefresh={onRefresh} isLoading={isLoading} error={error} />
        </div>
      )}
    </div>
  );
}

function CapabilitiesRowContent({
  profile,
  models,
  modes,
  commands,
  currentModelId,
  currentModeId,
  status,
  discoveryState,
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
  agentName,
  baselineProfile,
  disableUnverifiedModels,
}: CapabilitiesRowProps) {
  const activeMode = findActiveMode(modes, profile.mode, currentModeId);
  const labelCls = isCompact ? "text-xs" : undefined;
  const gapCls = isCompact ? "space-y-1.5" : "space-y-2";
  const showRefresh = status !== "auth_required" && status !== "not_installed";

  return (
    <div className={gapCls}>
      <ModelAndModePickers
        profile={profile}
        baselineProfile={baselineProfile}
        models={models}
        modes={modes}
        currentModelId={currentModelId}
        currentModeId={currentModeId}
        configOptions={configOptions}
        onChange={onChange}
        disableUnverifiedModels={disableUnverifiedModels}
        configIsLoading={configIsLoading}
        supportsDynamicModels={modelConfig.supports_dynamic_models}
        isLoading={isLoading}
        onRefresh={onRefresh}
        error={error}
        showRefresh={showRefresh}
        labelCls={labelCls}
        gapCls={gapCls}
      />
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
      <StatusOrAuthMessage
        agentName={agentName}
        status={status}
        isLoading={isLoading}
        onRefresh={onRefresh}
        error={error}
        discoveryState={discoveryState}
      />
    </div>
  );
}

type ProfileCapabilitiesSectionProps = {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
  modelConfig: ModelConfig;
  agentName: string;
  isCompact: boolean;
  result: ReturnType<typeof useProfileFormCapabilities>;
};

export function ProfileCapabilitiesSection({
  profile,
  baselineProfile,
  onChange,
  modelConfig,
  agentName,
  isCompact,
  result,
}: ProfileCapabilitiesSectionProps) {
  const {
    capabilities,
    configOptions,
    configStatus,
    configError,
    configIsLoading,
    refreshModelConfig,
    refresh,
    discoveryState,
  } = result;
  return (
    <CapabilitiesRow
      profile={profile}
      models={capabilities.models}
      modes={capabilities.modes}
      commands={capabilities.commands}
      currentModelId={capabilities.currentModelId}
      currentModeId={capabilities.currentModeId}
      status={capabilities.status}
      discoveryState={discoveryState}
      disableUnverifiedModels={discoveryState !== "ready"}
      agentName={agentName}
      onChange={onChange}
      isCompact={isCompact}
      isLoading={capabilities.isLoading}
      onRefresh={refresh}
      error={capabilities.error}
      modelConfig={modelConfig}
      configOptions={configOptions}
      configStatus={configStatus}
      configError={configError}
      configIsLoading={configIsLoading}
      onRetryConfig={refreshModelConfig}
      baselineProfile={baselineProfile}
    />
  );
}
