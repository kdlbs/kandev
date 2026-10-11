"use client";

import { useTranslation } from "react-i18next";
import { useOptionalAppStore } from "@/components/state-provider";
import { readAgentDeliveryHistoryGaps } from "@/lib/session-agent-delivery-recovery";

export function DeliveryHistoryNotice({ sessionId }: { sessionId: string | null }) {
  const { t } = useTranslation();
  const metadata = useOptionalAppStore(
    (state) => (sessionId ? state.taskSessions.items[sessionId]?.metadata : undefined),
    undefined,
  );
  if (!readAgentDeliveryHistoryGaps(metadata).length) return null;
  return (
    <p
      role="note"
      data-testid="delivery-history-notice"
      className="min-w-0 wrap-anywhere px-3 py-2 text-xs text-muted-foreground"
    >
      {t("task:deliveryHistoryGapArchived")}
    </p>
  );
}
