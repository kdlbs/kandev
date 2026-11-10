import { useCallback, useRef } from "react";

import { usableConfigOptions } from "@/components/model-config-selector";
import { resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
import { resolveSessionModelConfig } from "@/lib/api/domains/session-api";
import type { ProfileLaunchSettingsRequest } from "@/lib/types/http";
import type { ConfigurationOption } from "@/hooks/use-model-catalog-navigation";

export type CandidateModelOptionsInput = {
  agentName: string;
  /** Live session whose saved launch context should resolve the candidate. */
  sessionId?: string;
  profileId?: string;
  mode?: string;
  /** Complete draft launch snapshot. Omit to resolve with the saved profile. */
  launchSettings?: ProfileLaunchSettingsRequest;
};

/**
 * Resolves a candidate model's real provider options through the settings
 * probe. It never mutates the owning session or profile; the resolved values
 * only seed the picker's explicit Apply action.
 *
 * The callback stays stable while its inputs are unchanged so the focused
 * composition view does not refetch on every parent render.
 */
export function useCandidateModelOptions({
  agentName,
  sessionId,
  profileId,
  mode,
  launchSettings,
}: CandidateModelOptionsInput): (model: string) => Promise<ConfigurationOption[] | undefined> {
  const launchKey = JSON.stringify(launchSettings ?? null);
  const launchRef = useRef(launchSettings);
  launchRef.current = launchSettings;
  return useCallback(
    async (model: string) => {
      if (!agentName || !model) return undefined;
      const currentLaunch = launchRef.current;
      try {
        const response = sessionId
          ? await resolveSessionModelConfig(sessionId, {
              model,
              ...(mode ? { mode } : {}),
            })
          : await resolveAgentModelConfig(agentName, {
              model,
              ...(mode ? { mode } : {}),
              ...(profileId ? { profile_id: profileId } : {}),
              ...(currentLaunch ? { launch_settings: currentLaunch } : {}),
              refresh: true,
            });
        if (response.status !== "ok" || response.error) return undefined;
        return usableConfigOptions(
          (response.config_options ?? []).map((option) => ({
            type: option.type,
            id: option.id,
            name: option.name,
            description: option.description,
            currentValue: option.current_value,
            category: option.category,
            options: option.options,
            catalog: option.catalog,
          })),
        );
      } catch {
        return undefined;
      }
    },
    [agentName, sessionId, profileId, mode, launchKey],
  );
}
