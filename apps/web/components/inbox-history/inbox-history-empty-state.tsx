"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";

// Names what the bucket contains rather than a bare success message.
export function InboxHistoryEmptyState() {
  const { t } = useTranslation();
  const workspaceName = useAppStore(
    (s) => s.workspaces?.items?.find((w) => w.id === s.workspaces.activeId)?.name,
  );
  return (
    <div
      className="rounded-lg border border-border p-8 text-center"
      data-testid="inbox-history-empty"
    >
      <p className="text-sm font-medium">
        {workspaceName
          ? t("inboxHistory:emptyTitle", { workspace: workspaceName })
          : t("inboxHistory:emptyTitleUnnamedWorkspace")}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">{t("inboxHistory:emptyDescription")}</p>
    </div>
  );
}
