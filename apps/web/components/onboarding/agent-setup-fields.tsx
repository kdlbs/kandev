"use client";

import { useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import { ModelConfigSelector, type ModelSelectorOption } from "@/components/model-config-selector";
import {
  useProfileCapabilityDiscovery,
  type ProfileDiscoveryStatus,
} from "@/hooks/domains/settings/use-profile-capability-discovery";
import type { AgentSetting, OnboardingAgentDraft } from "@/components/onboarding/agent-settings";
import type { AvailableAgent, CapabilityStatus, ModelEntry } from "@/lib/types/http";
import { cn } from "@/lib/utils";

export type AgentSetupFieldsProps = {
  agent: AvailableAgent;
  setting: AgentSetting;
  onChange: (patch: Partial<OnboardingAgentDraft>) => void;
  onStatusChange?: (status: CapabilityStatus | undefined) => void;
};

function buildModelOptions(
  models: ModelEntry[],
  currentModel: string,
  discoveryState: ProfileDiscoveryStatus,
  unavailableLabel: string,
): { modelOptions: ModelSelectorOption[]; modelIsGone: boolean } {
  const options: ModelSelectorOption[] = models.map((model) => ({
    id: model.id,
    name: model.name,
    description: model.description || (model.id !== model.name ? model.id : undefined),
    usageMultiplier:
      typeof model.meta?.copilotUsage === "string" ? model.meta.copilotUsage : undefined,
  }));

  const modelIsGone = Boolean(
    discoveryState === "ready" &&
    models.length > 0 &&
    currentModel &&
    !options.some((m) => m.id === currentModel),
  );

  if (modelIsGone) {
    options.unshift({
      id: currentModel,
      name: currentModel,
      disabled: true,
      disabledReason: unavailableLabel,
    });
  }

  return { modelOptions: options, modelIsGone };
}

function ModelStatusFeedback({
  discoveryState,
  error,
  hasModels,
}: {
  discoveryState: ProfileDiscoveryStatus;
  error: string | null;
  hasModels: boolean;
}) {
  const { t } = useTranslation();

  if (discoveryState === "loading") {
    return (
      <p className="text-xs text-muted-foreground flex items-center gap-1" role="status">
        <IconLoader2 className="h-3 w-3 animate-spin shrink-0" />
        <span>{t("common:loading")}</span>
      </p>
    );
  }

  if (discoveryState === "failed") {
    return (
      <p className="text-xs text-destructive">{error || t("agents:failedToFetchCapabilities")}</p>
    );
  }

  if (discoveryState === "ready" && !hasModels) {
    return <p className="text-xs text-muted-foreground">{t("agents:noModelsFound")}</p>;
  }

  return null;
}

function PassthroughField({
  agent,
  checked,
  onChange,
}: {
  agent: AvailableAgent;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const config = agent.passthrough_config;
  if (!config?.supported) return null;

  const label =
    agent.name === "minimax-acp"
      ? t("agents:minimaxPassthroughLabel")
      : config.label || t("agents:minimaxPassthroughLabel");
  const description =
    agent.name === "minimax-acp" ? t("agents:minimaxPassthroughDescription") : config.description;

  return (
    <div
      className="flex items-center justify-between gap-2 pt-1"
      data-testid="onboarding-agent-passthrough-field"
    >
      <div className="space-y-0.5">
        <SettingsFieldLabel className="text-xs">{label}</SettingsFieldLabel>
        {description && (
          <SettingsFieldDescription className="text-xs text-muted-foreground">
            {description}
          </SettingsFieldDescription>
        )}
      </div>
      <Switch
        size="sm"
        checked={checked}
        onCheckedChange={(val) => onChange(val === true)}
        data-testid="onboarding-agent-passthrough-switch"
      />
    </div>
  );
}

function useProfileStatusSync(
  discoveryStatus: CapabilityStatus | undefined,
  discoveryState: ProfileDiscoveryStatus,
  onStatusChange?: (status: CapabilityStatus | undefined) => void,
) {
  useEffect(() => {
    if (discoveryStatus) {
      onStatusChange?.(discoveryStatus);
    } else if (discoveryState === "loading") {
      onStatusChange?.("probing");
    } else if (discoveryState === "failed") {
      onStatusChange?.("failed");
    } else if (discoveryState === "ready") {
      onStatusChange?.("ok");
    }
  }, [discoveryStatus, discoveryState, onStatusChange]);
}

export function AgentSetupFields({
  agent,
  setting,
  onChange,
  onStatusChange,
}: AgentSetupFieldsProps) {
  const { t } = useTranslation();

  const discovery = useProfileCapabilityDiscovery(agent.name, setting.savedLaunchSettings, {
    profileId: setting.profileId,
    savedLaunchSettings: setting.savedLaunchSettings,
    supportsDynamicModels: agent.model_config.supports_dynamic_models,
  });

  useProfileStatusSync(discovery.status, discovery.discoveryState, onStatusChange);

  const models = agent.model_config.supports_dynamic_models
    ? discovery.models
    : (agent.model_config.available_models ?? []);

  const currentModel =
    setting.draft.model || discovery.currentModelId || agent.model_config.default_model || "";

  const unavailableLabel = t("settings:startModelUnavailable");
  const { modelOptions, modelIsGone } = useMemo(
    () => buildModelOptions(models, currentModel, discovery.discoveryState, unavailableLabel),
    [models, currentModel, discovery.discoveryState, unavailableLabel],
  );

  const isBusy = discovery.discoveryState === "loading";
  const disableUnverifiedModels = isBusy || discovery.discoveryState === "failed";

  return (
    <div className="space-y-3 pt-1" data-testid="onboarding-agent-setup-fields">
      <div className="space-y-1.5" data-testid="onboarding-agent-model-field">
        <SettingsFieldLabel className="text-xs">{t("agents:startModel")}</SettingsFieldLabel>
        <div className="flex items-center gap-1.5">
          <div className="min-w-0 flex-1">
            <ModelConfigSelector
              modelOptions={modelOptions}
              currentModel={currentModel}
              onModelChange={(model) => onChange({ model })}
              placeholder={t("settings:selectAModel")}
              ariaLabel={t("settings:startModelAria")}
              popoverAlign="start"
              disabled={disableUnverifiedModels}
              triggerClassName={modelIsGone ? "text-destructive" : undefined}
            />
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <span tabIndex={isBusy ? 0 : -1} className="inline-flex">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={discovery.refresh}
                  disabled={isBusy}
                  aria-label={t("agents:refreshCapabilities")}
                  data-testid="onboarding-agent-refresh-models"
                  className="cursor-pointer shrink-0"
                >
                  <IconRefresh className={cn("h-4 w-4", isBusy && "animate-spin")} />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>{t("agents:refreshCapabilitiesTooltip")}</TooltipContent>
          </Tooltip>
        </div>
        <ModelStatusFeedback
          discoveryState={discovery.discoveryState}
          error={discovery.error}
          hasModels={models.length > 0}
        />
      </div>

      <PassthroughField
        agent={agent}
        checked={setting.draft.cli_passthrough}
        onChange={(cli_passthrough) => onChange({ cli_passthrough })}
      />
    </div>
  );
}
