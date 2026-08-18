"use client";

/**
 * Header duplicate action for the agent profile settings page, extracted so
 * the page stays presentation-only and the save-gate + in-flight guard are
 * unit-testable. Mirrors the agent-profile-page-state.ts split.
 */

import { useCallback, useRef, useState } from "react";
import { duplicateAgentProfileAction } from "@/app/actions/agents";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveCoordinator } from "@/components/settings/settings-save-provider";
import { applyProfileDuplicated } from "@/hooks/domains/settings/use-profile-duplicate";
import { errorMessage } from "@/components/settings/agent-profile-page-state";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import type { Agent, AgentProfile } from "@/lib/types/http";

/**
 * Returns the localized reason a profile draft cannot be saved yet (missing
 * name or model resolution still pending), or undefined when it is savable.
 */
export function profileSaveInvalidReason(
  profileName: string,
  modelConfigResolutionPending: boolean,
  translate: (key: string) => string,
): string | undefined {
  if (!profileName.trim()) return translate("agents:profileNameRequired");
  if (modelConfigResolutionPending) return translate("agents:resolvingModelOptions");
  return undefined;
}

type UseProfileDuplicateActionOptions = {
  agent: Agent;
  draft: AgentProfile;
  modelConfigResolutionPending: boolean;
  translate: (key: string) => string;
};

/**
 * Header "Duplicate" flow: save every dirty contributor (profile editor AND
 * MCP card) through the shared settings coordinator — aborting with the
 * blocking contributor's reason when anything is invalid — then POST the
 * duplicate, merge the copy into the store, toast, and SPA-navigate to the
 * copy's page. The in-flight guard is a ref (synchronous) so two rapid clicks
 * can never issue two copies; the state only drives the button's disabled
 * state.
 */
export function useProfileDuplicateAction({
  agent,
  draft,
  modelConfigResolutionPending,
  translate,
}: UseProfileDuplicateActionOptions) {
  const storeApi = useAppStoreApi();
  const router = useRouter();
  const { saveAll, invalidReason, cancelPendingNavigation } = useSettingsSaveCoordinator();
  const { toast } = useToast();
  const [duplicating, setDuplicating] = useState(false);
  const inFlightRef = useRef(false);

  const handleDuplicate = useCallback(async () => {
    if (inFlightRef.current) return;
    inFlightRef.current = true;
    setDuplicating(true);
    try {
      // Save EVERY dirty contributor (the profile editor AND the MCP card)
      // so the copy starts from what the user sees. saveAll respects each
      // contributor's canSave; when anything is invalid or fails to persist,
      // it reports canLeave=false and we abort before posting.
      const saved = await saveAll();
      if (!saved.canLeave) {
        // Surface whichever contributor is blocking the save (MCP error or
        // an invalid profile draft); when the coordinator gives no reason
        // (another save in flight, or newer changes appeared during the
        // awaited save), fall back to a generic message so the abort is
        // never silent.
        toast({
          title: translate("agents:failedToDuplicateProfile"),
          description:
            invalidReason ??
            profileSaveInvalidReason(draft.name, modelConfigResolutionPending, translate) ??
            translate("agents:duplicateSaveBlocked"),
          variant: "error",
        });
        return;
      }
      const created = await duplicateAgentProfileAction(draft.id);
      // Merge the copy into the store so the destination page resolves it
      // without waiting on the WS round-trip.
      storeApi.setState((state) => applyProfileDuplicated(state, agent, created));
      toast({
        title: translate("agents:duplicateProfileSuccess"),
        description: created.name,
        variant: "success",
      });
      // Supersede any navigation intent the dirty guard blocked (e.g. a
      // browser-back while dirty) — this duplicate navigation replaces it.
      cancelPendingNavigation();
      // SPA navigation, not location.assign: the toast survives the route
      // change and the dirty-settings blocker was already resolved by the
      // save above (the bypass is the pattern the executor editors use
      // after an awaited save).
      runWithNavigationBlockerBypassed(() =>
        router.push(`/settings/agents/${encodeURIComponent(agent.name)}/profiles/${created.id}`),
      );
    } catch (error) {
      toast({
        title: translate("agents:failedToDuplicateProfile"),
        description: errorMessage(error),
        variant: "error",
      });
    } finally {
      inFlightRef.current = false;
      setDuplicating(false);
    }
  }, [
    agent,
    cancelPendingNavigation,
    draft.id,
    draft.name,
    invalidReason,
    modelConfigResolutionPending,
    router,
    saveAll,
    storeApi,
    toast,
    translate,
  ]);

  return { handleDuplicate, duplicating };
}
