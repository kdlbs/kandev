"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";

// An empty Failed bucket is a normal state, not an achievement: no icon, no
// congratulation, and the workspace is named so the sentence is never read as
// a claim about the whole instance (AC-UI-INBOX-FAILED-001.21).
export function FailedInboxEmptyState() {
  const { t } = useTranslation();
  const workspaceName = useAppStore(
    (s) => s.workspaces?.items?.find((w) => w.id === s.workspaces.activeId)?.name,
  );
  return (
    <div
      className="rounded-lg border border-border p-8 text-center"
      data-testid="failed-inbox-empty"
    >
      <p className="text-sm font-medium">
        {workspaceName
          ? t("failedInbox:emptyTitle", { workspace: workspaceName })
          : t("failedInbox:emptyTitleUnnamedWorkspace")}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">{t("failedInbox:emptyDescription")}</p>
    </div>
  );
}
