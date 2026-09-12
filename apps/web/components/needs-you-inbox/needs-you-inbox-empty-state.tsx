"use client";

import { IconCircleCheck } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { NeedsYouInboxHiddenPanel } from "./needs-you-inbox-hidden-panel";

// Names what it does not count, and only mentions hidden bundles when the
// operator actually has some hidden -- it must never imply otherwise.
export function NeedsYouInboxEmptyState({ hiddenCount }: { hiddenCount: number }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div
        className="rounded-lg border border-border p-8 text-center"
        data-testid="needs-you-inbox-empty"
      >
        <IconCircleCheck className="mx-auto h-6 w-6 text-green-500" />
        <p className="mt-2 text-sm font-medium">{t("needsYouInbox:emptyTitle")}</p>
        <p className="mt-1 text-xs text-muted-foreground">{t("needsYouInbox:emptyDescription")}</p>
      </div>
      {hiddenCount > 0 && <NeedsYouInboxHiddenPanel hiddenCount={hiddenCount} />}
    </div>
  );
}
