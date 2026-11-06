"use client";

import { t } from "@/lib/i18n";
import { PlanCommentMigrationNotice } from "@/components/task/plan-comment-migration-notice";
import { ComposerAgentStartHint } from "./composer-agent-start-hint";
import { DynamicRouteRecovery } from "./dynamic-route-recovery";
import { SessionDeliveryStorageNotice } from "./session-delivery-storage-notice";
import type { ChatPanelState } from "./use-chat-panel-state";

/** Announces restart progress without showing the competing agent-start hint. */
export function ComposerStatusNotices({
  panelState,
  showAgentStartHint,
  executorUnavailable,
  isRestarting,
}: {
  panelState: ChatPanelState;
  showAgentStartHint: boolean;
  executorUnavailable: boolean;
  isRestarting: boolean;
}) {
  return (
    <>
      {isRestarting && (
        <p
          role="status"
          data-testid="session-restart-status"
          className="px-1 pb-1 text-xs text-muted-foreground"
        >
          {t("task:sessionActionRestart")}
        </p>
      )}
      <SessionDeliveryStorageNotice health={panelState.session?.runtime_storage_health} />
      <DynamicRouteRecovery session={panelState.session} />
      <ComposerAgentStartHint
        show={showAgentStartHint && !isRestarting}
        needsRecovery={panelState.needsRecovery}
        executorUnavailable={executorUnavailable}
        hasPendingClarification={panelState.hasPendingClarification}
      />
      <PlanCommentMigrationNotice {...panelState.planCommentMigration} />
    </>
  );
}
