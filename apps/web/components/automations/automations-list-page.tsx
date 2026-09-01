"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconPlus } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useAutomations } from "@/hooks/domains/settings/use-automations";
import { AutomationsTable } from "./automations-table";
import { AutomationBoardMoveNotice } from "./board-move-notice";
import { AutomationsExportButton } from "./automations-export-button";
import { useAutomationEnabledDrafts } from "./use-automation-enabled-drafts";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";
import { AutomationDeleteConfirmDialog } from "./automation-delete-confirm-dialog";
import type { Automation } from "@/lib/types/automation";

type AutomationsListPageProps = {
  workspaceId: string;
};

export function AutomationsListPage({ workspaceId }: AutomationsListPageProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const { items, loading, enable, disable, trigger, remove } = useAutomations(workspaceId);
  const enabledDrafts = useAutomationEnabledDrafts({ automations: items, enable, disable });
  const [automationToDelete, setAutomationToDelete] = useState<Automation | null>(null);
  const [deletingAutomationId, setDeletingAutomationId] = useState<string | null>(null);

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

  const handleDelete = (id: string) => {
    const automation = items.find((item) => item.id === id);
    if (automation) setAutomationToDelete(automation);
  };

  const handleConfirmDelete = async () => {
    if (!automationToDelete || deletingAutomationId) return;
    const { id } = automationToDelete;
    setDeletingAutomationId(id);
    try {
      await remove(id);
      setAutomationToDelete(null);
    } catch (error) {
      toast.error(t("automations:failedToDeleteAutomation"), {
        description: error instanceof Error ? error.message : t("common:requestFailed"),
      });
    } finally {
      setDeletingAutomationId(null);
    }
  };

  return (
    <div className="space-y-6" data-testid="automations-list-page">
      <WorkspaceSectionHeader
        tab="automations"
        description={t("automations:listDescription")}
        action={
          <div className="flex items-center gap-2">
            <AutomationsExportButton workspaceId={workspaceId} />
            <Button
              type="button"
              size="sm"
              data-testid="new-automation-button"
              className="min-h-11 cursor-pointer md:min-h-7"
              onClick={() => router.push(`/settings/workspaces/${workspaceId}/automations/new`)}
            >
              <IconPlus className="h-4 w-4 mr-2" />
              {t("automations:newAutomation")}
            </Button>
          </div>
        }
      />
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
      <AutomationDeleteConfirmDialog
        open={automationToDelete !== null}
        automationName={automationToDelete?.name ?? ""}
        isDeleting={deletingAutomationId !== null}
        onOpenChange={(open) => {
          if (!open && deletingAutomationId !== automationToDelete?.id) {
            setAutomationToDelete(null);
          }
        }}
        onConfirm={handleConfirmDelete}
      />
    </div>
  );
}
