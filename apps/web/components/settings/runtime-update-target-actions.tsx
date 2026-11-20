"use client";

import type { AgentUpdateStatus } from "@/lib/api";
import { useSettingsTargetActionRegistration } from "./settings-target-provider";

function RuntimeTargetAction({ targetId, activate }: { targetId: string; activate: () => void }) {
  useSettingsTargetActionRegistration(targetId, activate);
  return null;
}

/**
 * Fragment destinations for the runtime manager. Action registrations resolve
 * pending `/settings/agents#runtime-updates` and `#runtime-update-<agent>`
 * requests without a second hash listener or a hidden scroll proxy.
 */
export function RuntimeUpdateTargetActions({
  statuses,
  onOpenOverview,
  onOpenRuntime,
}: {
  statuses: AgentUpdateStatus[];
  onOpenOverview: () => void;
  onOpenRuntime: (agentName: string) => void;
}) {
  return (
    <>
      <RuntimeTargetAction targetId="runtime-updates" activate={onOpenOverview} />
      {statuses.map((status) => (
        <RuntimeTargetAction
          key={`${status.agent_name}:${status.runtime_id}`}
          targetId={`runtime-update-${status.agent_name}`}
          activate={() => onOpenRuntime(status.agent_name)}
        />
      ))}
    </>
  );
}
