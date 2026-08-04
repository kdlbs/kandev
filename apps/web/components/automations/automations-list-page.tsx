"use client";

import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconPlus, IconBolt } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useAutomations } from "@/hooks/domains/settings/use-automations";
import { AutomationsTable } from "./automations-table";
import { AutomationBoardMoveNotice } from "./board-move-notice";
import { useAutomationEnabledDrafts } from "./use-automation-enabled-drafts";

type AutomationsListPageProps = {
  workspaceId: string;
};

export function AutomationsListPage({ workspaceId }: AutomationsListPageProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const { items, loading, enable, disable, trigger, remove } = useAutomations(workspaceId);
  const enabledDrafts = useAutomationEnabledDrafts({ automations: items, enable, disable });

  const handleTrigger = async (id: string) => {
    // Triggering can legitimately do nothing — the concurrency cap turns the
    // request away while an earlier run is still going. Saying so is the whole
    // point of the click having any feedback at all.
    try {
      const result = await trigger(id);
      if (result?.skipped) {
        toast.info(
          result.reason
            ? t("automations:skipped", { reason: result.reason })
            : t("automations:skippedAlreadyRunning"),
        );
        return;
      }
      toast.success(t("automations:triggered"));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("automations:couldNotTrigger"));
    }
  };

  const handleDelete = async (id: string) => {
    await remove(id);
  };

  return (
    <div className="space-y-6" data-testid="automations-list-page">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold flex items-center gap-2">
            <IconBolt className="h-5 w-5" />
            {t("common:automations")}
          </h2>
          <p className="text-sm text-muted-foreground">{t("automations:listDescription")}</p>
        </div>
        <Button
          data-testid="new-automation-button"
          className="cursor-pointer"
          onClick={() => router.push(`/settings/workspace/${workspaceId}/automations/new`)}
        >
          <IconPlus className="h-4 w-4 mr-1" />
          {t("automations:newAutomation")}
        </Button>
      </div>
      <Separator />
      {/* Above the table on purpose: it explains why the table's automations
          stopped showing up where the reader last saw them, so it has to be
          read before the table, not after it. */}
      <AutomationBoardMoveNotice workspaceId={workspaceId} automations={items} />
      {loading && items.length === 0 ? (
        <div className="py-12 text-center text-muted-foreground">
          {t("automations:loadingAutomations")}
        </div>
      ) : (
        <AutomationsTable
          automations={enabledDrafts.automations}
          dirtyIds={enabledDrafts.dirtyIds}
          workspaceId={workspaceId}
          onToggleEnabled={enabledDrafts.setEnabled}
          onTrigger={handleTrigger}
          onDelete={handleDelete}
        />
      )}
    </div>
  );
}
