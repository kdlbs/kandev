"use client";

import { PlanCommentMigrationNotice } from "@/components/task/plan-comment-migration-notice";
import { ComposerAgentStartHint } from "./composer-agent-start-hint";
import { DynamicRouteRecovery } from "./dynamic-route-recovery";
import type { ChatPanelState } from "./use-chat-panel-state";

export function ComposerStatusNotices({
  panelState,
  showAgentStartHint,
  executorUnavailable,
}: {
  panelState: ChatPanelState;
  showAgentStartHint: boolean;
  executorUnavailable: boolean;
}) {
  return (
    <>
      <DynamicRouteRecovery session={panelState.session} />
      <ComposerAgentStartHint
        show={showAgentStartHint}
        needsRecovery={panelState.needsRecovery}
        executorUnavailable={executorUnavailable}
        hasPendingClarification={Boolean(panelState.pendingClarification)}
      />
      <PlanCommentMigrationNotice {...panelState.planCommentMigration} />
    </>
  );
}
