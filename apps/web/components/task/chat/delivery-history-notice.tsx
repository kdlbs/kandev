"use client";

import { useTranslation } from "react-i18next";
import { useOptionalAppStore } from "@/components/state-provider";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import { readAgentDeliveryHistoryGaps } from "@/lib/session-agent-delivery-recovery";

export function DeliveryHistoryNotice({ sessionId }: { sessionId: string | null }) {
  const { t } = useTranslation();
  const metadata = useOptionalAppStore(
    (state) => (sessionId ? state.taskSessions.items[sessionId]?.metadata : undefined),
    undefined,
  );
  const gaps = readAgentDeliveryHistoryGaps(metadata);
  if (!gaps.length) return null;
  const details = gaps
    .map((gap) =>
      [
        gap.streamId,
        t("task:deliveryHistoryGapRange", { first: gap.firstSequence, last: gap.lastSequence }),
      ].join("\n"),
    )
    .join("\n\n");
  return (
    <div
      role="note"
      data-testid="delivery-history-notice"
      className="min-w-0 wrap-anywhere px-3 py-2 text-xs text-muted-foreground"
    >
      <p>{t("task:deliveryHistoryGapArchived")}</p>
      <SessionErrorDetails testId="delivery-history-details">{details}</SessionErrorDetails>
    </div>
  );
}
