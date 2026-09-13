"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { NeedsYouInboxHiddenPanel } from "./needs-you-inbox-hidden-panel";

// An empty queue is a normal state, not an achievement: no icon, no
// congratulation, and the workspace is named so the sentence is never read as a
// claim about the whole instance (design-03#D2). Hidden bundles are disclosed
// only when the operator actually has some (AC .20).
export function NeedsYouInboxEmptyState({ hiddenCount }: { hiddenCount: number }) {
  const { t } = useTranslation();
  const workspaceName = useAppStore(
    (s) => s.workspaces?.items?.find((w) => w.id === s.workspaces.activeId)?.name,
  );
  return (
    <div className="space-y-4">
      <div
        className="rounded-lg border border-border p-8 text-center"
        data-testid="needs-you-inbox-empty"
      >
        <p className="text-sm font-medium">
          {workspaceName
            ? t("needsYouInbox:emptyTitle", { workspace: workspaceName })
            : t("needsYouInbox:emptyTitleUnnamedWorkspace")}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">{t("needsYouInbox:emptyDescription")}</p>
      </div>
      {hiddenCount > 0 && <NeedsYouInboxHiddenPanel hiddenCount={hiddenCount} />}
    </div>
  );
}
