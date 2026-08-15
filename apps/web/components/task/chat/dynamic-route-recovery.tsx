"use client";

import { useCallback, useState } from "react";
import { IconAlertTriangle, IconArrowRight, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import { getWebSocketClient } from "@/lib/ws/connection";

const ROUTE_REASON_KEYS: Record<string, string> = {
  candidate_order: "dynamicRouteReasonCandidateOrder",
  no_eligible_candidate: "dynamicRouteReasonNoEligibleCandidate",
  provider_unavailable: "dynamicRouteReasonProviderUnavailable",
  quota_limited: "dynamicRouteReasonQuotaLimited",
};

type RouteAction = "retry" | "try_next";
type RouteActionResult = {
  execution_profile_id?: string;
  route_generation: number;
  state: string;
  reason?: string;
};

async function applyRouteAction(
  session: TaskSession,
  action: RouteAction,
): Promise<RouteActionResult | null> {
  const client = getWebSocketClient();
  if (!client || session.route_generation === undefined) return null;
  try {
    const result = await client.request<RouteActionResult>(
      "session.route_action",
      {
        session_id: session.id,
        action,
        expected_generation: session.route_generation,
      },
      30000,
    );
    return result;
  } catch {
    return null;
  }
}

export function DynamicRouteRecovery({ session }: { session: TaskSession | null }) {
  const { t } = useTranslation("task");
  const [pendingAction, setPendingAction] = useState<RouteAction | null>(null);
  const updateSession = useAppStore((state) => state.setTaskSession);

  const handleAction = useCallback(
    async (action: RouteAction) => {
      if (!session) return;
      setPendingAction(action);
      const result = await applyRouteAction(session, action);
      setPendingAction(null);
      if (result) {
        updateSession({
          ...session,
          execution_profile_id: result.execution_profile_id
            ? toAgentProfileId(result.execution_profile_id)
            : undefined,
          route_generation: result.route_generation,
          route_state: result.state,
          route_reason: result.reason,
          updated_at: new Date().toISOString(),
        });
      }
    },
    [session, updateSession],
  );

  if (
    !session ||
    !session.route_state ||
    (session.route_state !== "waiting" && session.route_state !== "action_required") ||
    session.route_generation === undefined
  ) {
    return null;
  }

  const isWaiting = session.route_state === "waiting";
  const reasonKey = ROUTE_REASON_KEYS[session.route_reason ?? ""];

  return (
    <section
      className="mb-2 flex flex-col gap-3 rounded-md border border-border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid="dynamic-route-recovery"
    >
      <div className="flex min-w-0 items-start gap-2 text-sm">
        <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-orange-500" aria-hidden="true" />
        <div className="min-w-0">
          <p className="font-medium">
            {t(isWaiting ? "dynamicRouteWaiting" : "dynamicRouteActionRequired")}
          </p>
          {reasonKey && <p className="truncate text-muted-foreground">{t(reasonKey)}</p>}
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          className="min-h-11 min-w-11 gap-1.5"
          disabled={pendingAction !== null}
          onClick={() => void handleAction("retry")}
          data-testid="dynamic-route-retry"
        >
          <IconRefresh className="h-4 w-4" aria-hidden="true" />
          {t(pendingAction === "retry" ? "dynamicRouteRetrying" : "dynamicRouteRetry")}
        </Button>
        <Button
          type="button"
          variant="default"
          className="min-h-11 min-w-11 gap-1.5"
          disabled={pendingAction !== null}
          onClick={() => void handleAction("try_next")}
          data-testid="dynamic-route-try-next"
        >
          <IconArrowRight className="h-4 w-4" aria-hidden="true" />
          {t(pendingAction === "try_next" ? "dynamicRouteTryNexting" : "dynamicRouteTryNext")}
        </Button>
      </div>
    </section>
  );
}
