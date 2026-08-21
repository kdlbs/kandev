import { useMemo } from "react";

import { useFeature } from "@/hooks/domains/features/use-feature";
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { isHealthyAgentProfile } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import {
  isAgentConfiguredOnExecutor,
  shouldFilterHandoffByHostHealth,
} from "@/lib/agent-executor-compat";
import type { AgentProfileOption } from "@/lib/state/slices";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import type { ExecutorProfile } from "@/lib/types/http";
import type { HandoffPreset } from "@/components/task/handoff-types";

export function useCompatibleAgentProfiles(
  agentProfiles: AgentProfileOption[],
  executorProfile: ExecutorProfile | null,
  handoff?: HandoffPreset,
): AgentProfileOption[] {
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();
  const filterByHostHealth = Boolean(handoff && shouldFilterHandoffByHostHealth(executorProfile));

  return useMemo(() => {
    const selectable = agentProfiles.filter(
      (profile) =>
        isSelectableAgentProfile(profile, dynamicRoutingEnabled) &&
        (!filterByHostHealth || isHealthyAgentProfile(profile)),
    );
    if (!executorProfile || !authLoaded) return selectable;
    return selectable.filter((profile) =>
      isAgentConfiguredOnExecutor(profile, executorProfile, authSpecs),
    );
  }, [
    agentProfiles,
    dynamicRoutingEnabled,
    executorProfile,
    authSpecs,
    authLoaded,
    filterByHostHealth,
  ]);
}
