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

type RouteAction = "retry" | "try_next" | "skip" | "cancel_wait" | "stop";
type RouteActionResult = {
  execution_profile_id?: string;
  route_generation: number;
  state: string;
  reason?: string;
  error_code?: string;
  error_class?: string;
  catalogue_version?: string;
  retry_ordinal?: number;
  deadline?: string;
  pending_outcome?: string;
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

function applyRouteActionResult(session: TaskSession, result: RouteActionResult): TaskSession {
  return {
    ...session,
    execution_profile_id: result.execution_profile_id
      ? toAgentProfileId(result.execution_profile_id)
      : undefined,
    route_generation: result.route_generation,
    route_state: result.state,
    route_reason: result.reason,
    route_error_code: result.error_code,
    route_error_class: result.error_class,
    route_catalogue_version: result.catalogue_version,
    route_retry_ordinal: result.retry_ordinal,
    route_deadline: result.deadline,
    route_pending_outcome: result.pending_outcome,
    updated_at: new Date().toISOString(),
  };
}

function DynamicRouteRecoverySummary({ session }: { session: TaskSession }) {
  const { t } = useTranslation("task");
  const isPolicyPending =
    session.route_state === "waiting" ||
    session.route_state === "waiting_for_reset" ||
    session.route_state === "retry_wait";
  const reasonKey = ROUTE_REASON_KEYS[session.route_reason ?? ""];
  const deadline = session.route_deadline ? new Date(session.route_deadline) : null;
  const deadlineText =
    deadline && !Number.isNaN(deadline.getTime())
      ? new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
          deadline,
        )
      : null;

  return (
    <div className="flex min-w-0 items-start gap-2 text-sm">
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-orange-500" aria-hidden="true" />
      <div className="min-w-0">
        <p className="font-medium">
          {t(isPolicyPending ? "dynamicRouteWaiting" : "dynamicRouteActionRequired")}
        </p>
        {reasonKey && <p className="truncate text-muted-foreground">{t(reasonKey)}</p>}
        {session.route_error_class && (
          <p className="text-muted-foreground">
            {t("dynamicRouteErrorClass", { class: session.route_error_class })}
          </p>
        )}
        {session.route_error_code && (
          <p className="text-muted-foreground">
            {t("dynamicRouteErrorCode", { code: session.route_error_code })}
          </p>
        )}
        {session.route_retry_ordinal !== undefined && session.route_retry_ordinal > 0 && (
          <p className="text-muted-foreground">
            {t("dynamicRouteRetryOrdinal", { count: session.route_retry_ordinal })}
          </p>
        )}
        {deadlineText && (
          <p className="text-muted-foreground">
            {t("dynamicRouteDeadline", { deadline: deadlineText })}
          </p>
        )}
        {session.route_pending_outcome && (
          <p className="text-muted-foreground">
            {t("dynamicRoutePendingOutcome", { outcome: session.route_pending_outcome })}
          </p>
        )}
      </div>
    </div>
  );
}

function DynamicRouteRecoveryActions({
  pendingAction,
  onAction,
  pendingWait,
}: {
  pendingAction: RouteAction | null;
  onAction: (action: RouteAction) => void;
  pendingWait: boolean;
}) {
  const { t } = useTranslation("task");

  return (
    <div className="flex flex-wrap gap-2">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 min-w-11 gap-1.5"
        disabled={pendingAction !== null}
        onClick={() => onAction("retry")}
        data-testid="dynamic-route-retry"
      >
        <IconRefresh className="h-4 w-4" aria-hidden="true" />
        {t(pendingAction === "retry" ? "dynamicRouteRetrying" : "dynamicRouteRetryNow")}
      </Button>
      <Button
        type="button"
        variant="default"
        className="min-h-11 min-w-11 gap-1.5"
        disabled={pendingAction !== null}
        onClick={() => onAction("skip")}
        data-testid="dynamic-route-try-next"
      >
        <IconArrowRight className="h-4 w-4" aria-hidden="true" />
        {t(pendingAction === "skip" ? "dynamicRouteSkipping" : "dynamicRouteSkipNow")}
      </Button>
      {pendingWait && (
        <Button
          type="button"
          variant="outline"
          className="min-h-11 min-w-11"
          disabled={pendingAction !== null}
          onClick={() => onAction("cancel_wait")}
          data-testid="dynamic-route-cancel-wait"
        >
          {t("dynamicRouteCancelWait")}
        </Button>
      )}
      <Button
        type="button"
        variant="destructive"
        className="min-h-11 min-w-11"
        disabled={pendingAction !== null}
        onClick={() => onAction("stop")}
        data-testid="dynamic-route-stop"
      >
        {t("dynamicRouteStop")}
      </Button>
    </div>
  );
}

export function DynamicRouteRecovery({ session }: { session: TaskSession | null }) {
  const [pendingAction, setPendingAction] = useState<RouteAction | null>(null);
  const updateSession = useAppStore((state) => state.setTaskSession);

  const handleAction = useCallback(
    async (action: RouteAction) => {
      if (!session) return;
      setPendingAction(action);
      const result = await applyRouteAction(session, action);
      setPendingAction(null);
      if (result) {
        updateSession(applyRouteActionResult(session, result));
      }
    },
    [session, updateSession],
  );

  if (
    !session ||
    !session.route_state ||
    !["waiting", "waiting_for_reset", "retry_wait", "retrying", "action_required"].includes(
      session.route_state,
    ) ||
    session.route_generation === undefined
  ) {
    return null;
  }
  const pendingWait =
    session.route_state === "waiting_for_reset" || session.route_state === "retry_wait";

  return (
    <section
      className="mb-2 flex flex-col gap-3 rounded-md border border-border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid="dynamic-route-recovery"
    >
      <DynamicRouteRecoverySummary session={session} />
      <DynamicRouteRecoveryActions
        pendingAction={pendingAction}
        pendingWait={pendingWait}
        onAction={(action) => void handleAction(action)}
      />
    </section>
  );
}
