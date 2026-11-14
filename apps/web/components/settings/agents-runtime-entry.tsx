"use client";

import type { RefObject } from "react";
import type { AgentUpdateJob, AgentUpdateStatus, InstallJob } from "@/lib/api";
import {
  buildRuntimeUpdateManagerModel,
  runtimePolicyKey,
} from "@/lib/agents/runtime-update-manager";
import { useRuntimePolicyControllers } from "./agent-runtime-policies";
import { RuntimeUpdatesButton } from "./runtime-updates-button";

/**
 * Computes the visible availability badge from the same model as the manager.
 * Unknown checks, external guidance, unavailable runtimes, and busy agents do
 * not inflate the count.
 */
export function AgentsRuntimeEntry({
  statuses,
  installJobs,
  updateJobs,
  hasRuntimeControl,
  isMobile,
  onOpen,
  buttonRef,
}: {
  statuses: AgentUpdateStatus[];
  installJobs: Record<string, InstallJob>;
  updateJobs: Record<string, AgentUpdateJob>;
  hasRuntimeControl: (agentName: string) => boolean;
  isMobile: boolean;
  onOpen: (opener: HTMLElement) => void;
  buttonRef?: RefObject<HTMLButtonElement | null>;
}) {
  const controllers = useRuntimePolicyControllers();
  const dirtyPolicyKeys = new Set(
    controllers
      .filter((controller) => controller.isDirty)
      .map((controller) => runtimePolicyKey(controller.status)),
  );
  const model = buildRuntimeUpdateManagerModel(statuses, {
    hasRuntimeControl,
    installJobs,
    updateJobs,
    dirtyPolicyKeys,
  });
  return (
    <RuntimeUpdatesButton
      count={model.availableCount}
      isMobile={isMobile}
      onClick={onOpen}
      buttonRef={buttonRef}
    />
  );
}
