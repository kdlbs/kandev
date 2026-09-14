"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { toast } from "@/lib/toast/sonner";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { selectOfficeAgentProfile } from "@/lib/state/slices/office/selectors";
import { updateAgentStatus } from "@/lib/api/domains/office-api";
import { cn } from "@/lib/utils";
import type { AgentStatus } from "@/lib/state/slices/office/types";

const RECOVERABLE_STATUSES: ReadonlySet<AgentStatus> = new Set(["paused", "stopped"]);

type Props = { agentId: string };

/**
 * Recovery control for an out-of-service Office agent. One control, one
 * accessible name, shown for `paused` and `stopped` alike — the target
 * status requested is always the constant `idle`, never derived from the
 * rendered status. The rendered status is sent as an expected recoverable
 * status so a stale browser cannot clear a live working owner.
 *
 * Reads the agent from the store by id, rather than taking a resolved agent
 * prop, so a store patch after recovery re-renders this control immediately.
 */
export function AgentRecoveryControl({ agentId }: Props) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const agent = useAppStore((s) => selectOfficeAgentProfile(s, agentId));
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);
  const storeApi = useAppStoreApi();
  const [recovering, setRecovering] = useState(false);

  const handleRecover = useCallback(async () => {
    if (!agent) return;
    const requestedFromStatus = agent.status;
    setRecovering(true);
    try {
      const updated = await updateAgentStatus(agent.id, "idle", {
        expectedStatus: requestedFromStatus,
      });
      // Something other than this request (a WS-triggered refetch, another
      // client's write) may have moved the store row while the request was
      // in flight. Applying this response then would clobber a newer status
      // with the one it superseded, so only patch the store if the row is
      // still exactly where this request left it.
      const current = selectOfficeAgentProfile(storeApi.getState(), agent.id);
      if (current?.status === requestedFromStatus) {
        updateStore(agent.workspaceId as string, agent.id, {
          status: updated.status,
          pauseReason: updated.pauseReason,
        });
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToReturnAgentToService"));
    } finally {
      setRecovering(false);
    }
  }, [agent, storeApi, t, updateStore]);

  if (!agent?.status || !RECOVERABLE_STATUSES.has(agent.status)) return null;

  return (
    <div className="flex items-center gap-2 min-w-0">
      {agent.pauseReason && (
        <span
          data-testid="agent-pause-reason"
          className="truncate text-xs text-muted-foreground"
          title={agent.pauseReason}
        >
          {agent.pauseReason}
        </span>
      )}
      <Button
        size="sm"
        variant="outline"
        data-testid="agent-recovery-control"
        onClick={handleRecover}
        disabled={recovering}
        aria-busy={recovering}
        className={cn("cursor-pointer shrink-0", !isFinePointer && "min-h-11")}
      >
        {recovering ? t("office:returningAgentToService") : t("office:returnAgentToService")}
      </Button>
    </div>
  );
}
