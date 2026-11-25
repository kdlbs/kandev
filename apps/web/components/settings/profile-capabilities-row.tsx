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
import { ProfileThinkingPicker } from "./profile-thinking-picker";
import { ModelDiscoveryNote } from "@/components/settings/model-discovery-note";
import { AntigravityRuntimeLabel } from "@/components/settings/antigravity-runtime-label";
import type { ProfileDiscoveryStatus } from "@/hooks/domains/settings/use-profile-model-capabilities";
import type {
  CommandEntry,
  ModelConfig,
  ModeEntry,
  ModelEntry,
  ModelDiscovery,
} from "@/lib/types/http";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import type { ProfileFormData } from "./profile-form-fields";
import type { useProfileFormCapabilities } from "./profile-capability-helpers";

type CapabilitiesRowProps = {
  discovery?: ModelDiscovery;
  profile: ProfileFormData;
  models: ModelEntry[];
  modes: ModeEntry[];
  commands: CommandEntry[];
  currentModelId: string | undefined;
  currentModeId: string | undefined;
  status: ModelConfig["status"];
  reasonCode?: string;
  discoveryState: ProfileDiscoveryStatus;
  disableUnverifiedModels: boolean;
  onChange: (patch: Partial<ProfileFormData>) => void;
  isCompact: boolean;
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  runtimeVariant?: ModelConfig["runtime_variant"];
  modelConfig: ModelConfig;
  configOptions: SelectConfigOption[];
  configStatus: ModelConfig["status"];
  configReasonCode?: string;
  configError: string | null;
  configIsLoading: boolean;
  onRetryConfig: () => Promise<void>;
  agentName: string;
  baselineProfile?: ProfileFormData;
  resolveConfigurationOptions?: ModelConfigResolver;
  onConfigurationApply?: ModelConfigApply;
};

type ModelConfigResolver = NonNullable<
  import("@/components/model-config-selector").ModelConfigSelectorProps["resolveConfigurationOptions"]
>;
type ModelConfigApply = NonNullable<
  import("@/components/model-config-selector").ModelConfigSelectorProps["onConfigurationApply"]
>;

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
  reasonCode,
  discoveryState,
  runtimeVariant,
}: {
  agentName: string;
  status: ModelConfig["status"];
  isLoading: boolean;
  onRefresh: () => Promise<void>;
  error: string | null;
  reasonCode?: string;
  discoveryState: ProfileDiscoveryStatus;
  runtimeVariant?: ModelConfig["runtime_variant"];
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
        runtimeVariant={runtimeVariant}
      />
    );
  }
  return <CapabilityStatusMessage status={discoveryState} reasonCode={reasonCode} />;
}

type ModelAndModePickersProps = {
  agentName: string;
  discovery?: ModelDiscovery;
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
  resolveConfigurationOptions?: ModelConfigResolver;
  onConfigurationApply?: ModelConfigApply;
};

function ProfileModeControl({
  agentName,
  profile,
  configOptions,
  modes,
  currentModeId,
  onChange,
  disableUnverifiedModels,
  configIsLoading,
}: Pick<
  ModelAndModePickersProps,
  | "agentName"
  | "profile"
  | "configOptions"
  | "modes"
  | "currentModeId"
  | "onChange"
  | "disableUnverifiedModels"
  | "configIsLoading"
>) {
  return agentName === "pi-acp" ? (
    <ProfileThinkingPicker
      profile={profile}
      options={configOptions}
      modes={modes}
      onChange={onChange}
      disabled={disableUnverifiedModels || configIsLoading}
    />
  ) : (
    <ModePicker
      profile={profile}
      modes={modes}
      currentModeId={currentModeId}
      onChange={onChange}
      disabled={disableUnverifiedModels}
    />
  );
}

function ModelAndModePickers({
  agentName,
  discovery,
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
  resolveConfigurationOptions,
  onConfigurationApply,
}: ModelAndModePickersProps) {
  const { t } = useTranslation();
  const isPi = agentName === "pi-acp";
  const hasModes = isPi || modes.length > 0;
  const showModePicker =
    hasModes || Boolean(profile.mode) || Boolean(currentModeId) || !disableUnverifiedModels;

  return (
    <div
      className={
        showModePicker
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
            discovery={discovery}
            profile={profile}
            models={models}
            currentModelId={currentModelId}
            configOptions={
              isPi
                ? configOptions.filter((option) => option.category !== "thought_level")
                : configOptions
            }
            onChange={onChange}
            ariaLabel={t("settings:startModelAria")}
            goneModelLabel={t("settings:startModelUnavailable")}
            disabled={disableUnverifiedModels}
            configOptionsLoading={configIsLoading}
            keepOpenOnModelChange={supportsDynamicModels}
            resolveConfigurationOptions={resolveConfigurationOptions}
            onConfigurationApply={onConfigurationApply}
          />
        </div>
      </div>
      {showModePicker && (
        <div
          data-testid="profile-mode-field"
          className={`col-span-2 row-start-2 min-w-0 md:col-span-1 md:col-start-2 md:row-start-1 ${gapCls}`}
          data-settings-dirty={profileModeIsDirty(profile, baselineProfile)}
          data-settings-dirty-level="container"
        >
          <SettingsFieldLabel className={labelCls}>
            {t(isPi ? "agents:thinkingLevel" : "agents:startMode")}
          </SettingsFieldLabel>
          <ProfileModeControl
            agentName={agentName}
            profile={profile}
            configOptions={configOptions}
            modes={modes}
            currentModeId={currentModeId}
            onChange={onChange}
            disableUnverifiedModels={disableUnverifiedModels}
            configIsLoading={configIsLoading}
          />
        </div>
      )}
      {showRefresh && (
        <div
          className={`col-start-2 row-start-1 shrink-0 ${showModePicker ? "md:col-start-3" : ""}`}
        >
          <RefreshCapabilitiesButton onRefresh={onRefresh} isLoading={isLoading} error={error} />
        </div>
      )}
    </div>
  );
}

function CapabilitiesRowContent({
  discovery,
  profile,
  models,
  modes,
  commands,
  currentModelId,
  currentModeId,
  status,
  reasonCode,
  discoveryState,
  onChange,
  isCompact,
  isLoading,
  onRefresh,
  error,
  runtimeVariant,
  modelConfig,
  configOptions,
  configStatus,
  configReasonCode,
  configError,
  configIsLoading,
  onRetryConfig,
  agentName,
  baselineProfile,
  disableUnverifiedModels,
  resolveConfigurationOptions,
  onConfigurationApply,
}: CapabilitiesRowProps) {
  const activeMode = findActiveMode(modes, profile.mode, currentModeId);
  const labelCls = isCompact ? "text-xs" : undefined;
  const gapCls = isCompact ? "space-y-1.5" : "space-y-2";
  const showRefresh = status !== "auth_required" && status !== "not_installed";

  return (
    <div className={gapCls}>
      {agentName === "antigravity-acp" && <AntigravityRuntimeLabel variant={runtimeVariant} />}
      <ModelAndModePickers
        agentName={agentName}
        discovery={discovery}
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
        error={status === "unsupported" && reasonCode === "policy_unverified" ? null : error}
        showRefresh={showRefresh}
        labelCls={labelCls}
        gapCls={gapCls}
        resolveConfigurationOptions={resolveConfigurationOptions}
        onConfigurationApply={onConfigurationApply}
      />
      <ModelDiscoveryNote discovery={discovery} />
      <ModelConfigResolutionStatus
        status={configStatus}
        reasonCode={configReasonCode}
        error={configError}
        isLoading={configIsLoading}
        onRetry={onRetryConfig}
      />
      {agentName !== "pi-acp" && activeMode?.description && (
        <SettingsFieldDescription>{activeMode.description}</SettingsFieldDescription>
      )}
      {commands.length > 0 && <CommandsButton commands={commands} />}
      <StatusOrAuthMessage
        agentName={agentName}
        status={status}
        isLoading={isLoading}
        onRefresh={onRefresh}
        error={error}
        reasonCode={reasonCode}
        discoveryState={discoveryState}
        runtimeVariant={runtimeVariant}
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
  resolveConfigurationOptions?: ModelConfigResolver;
  onConfigurationApply?: ModelConfigApply;
};

export function ProfileCapabilitiesSection({
  profile,
  baselineProfile,
  onChange,
  modelConfig,
  agentName,
  isCompact,
  result,
  resolveConfigurationOptions,
  onConfigurationApply,
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
      discovery={capabilities.discovery}
      modes={capabilities.modes}
      commands={capabilities.commands}
      currentModelId={capabilities.currentModelId}
      currentModeId={capabilities.currentModeId}
      status={capabilities.status}
      reasonCode={capabilities.reasonCode}
      discoveryState={discoveryState}
      disableUnverifiedModels={discoveryState !== "ready"}
      agentName={agentName}
      onChange={onChange}
      isCompact={isCompact}
      isLoading={capabilities.isLoading}
      onRefresh={refresh}
      error={capabilities.error}
      runtimeVariant={capabilities.runtimeVariant}
      modelConfig={modelConfig}
      configOptions={configOptions}
      configStatus={configStatus}
      configReasonCode={result.configReasonCode}
      configError={configError}
      configIsLoading={configIsLoading}
      onRetryConfig={refreshModelConfig}
      baselineProfile={baselineProfile}
      resolveConfigurationOptions={resolveConfigurationOptions}
      onConfigurationApply={onConfigurationApply}
    />
  );
}
