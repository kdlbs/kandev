"use client";

import { useCallback, useState } from "react";
import { IconBellRinging, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { SettingsSection } from "@/components/settings/settings-section";
import { WatcherSettingsCard } from "@/components/integrations/watcher-settings-card";
import { useToast } from "@/components/toast-provider";
import { useJiraIssueWatches } from "@/hooks/domains/jira/use-jira-issue-watches";
import { useWatcherEnabledDrafts } from "@/components/integrations/use-watcher-enabled-drafts";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { JiraIssueWatchTable } from "./jira-issue-watch-table";
import { JiraIssueWatchDialog } from "./jira-issue-watch-dialog";
import type { JiraIssueWatch } from "@/lib/types/jira";
import { useTranslation } from "react-i18next";

// JiraIssueWatchersSection lists watches across every workspace in a single
// flat table on the install-wide settings page. The dialog's create flow asks
// the user to pick the workspace; per-row mutations forward each watch's
// stored workspaceId so the backend's IDOR guard accepts them.
type RawActions = {
  create: ReturnType<typeof useJiraIssueWatches>["create"];
  update: ReturnType<typeof useJiraIssueWatches>["update"];
  remove: ReturnType<typeof useJiraIssueWatches>["remove"];
  trigger: ReturnType<typeof useJiraIssueWatches>["trigger"];
  reset: ReturnType<typeof useJiraIssueWatches>["reset"];
};

// useToastedActions wraps the raw create/update/delete/trigger callbacks with
// success/failure toasts. Per-row mutations need each watch's own workspaceId,
// so the wrappers take the watch (not just an id) and pass it through.
function useToastedActions({ create, update, remove, trigger, reset }: RawActions) {
  const { t } = useTranslation();
  const { toast } = useToast();

  const wrappedCreate = useCallback(
    async (req: Parameters<typeof create>[0]) => {
      try {
        await create(req);
        toast({ description: t("jira:watcherCreated"), variant: "success" });
      } catch (err) {
        toast({ description: t("jira:createFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [create, t, toast],
  );

  const wrappedUpdate = useCallback(
    async (id: string, req: Parameters<typeof update>[1], rowWorkspaceId: string) => {
      try {
        await update(id, req, rowWorkspaceId);
        toast({ description: t("jira:watcherUpdated"), variant: "success" });
      } catch (err) {
        toast({ description: t("jira:updateFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [update, t, toast],
  );

  const wrappedDelete = useCallback(
    async (w: JiraIssueWatch) => {
      try {
        await remove(w.id, w.workspaceId);
        toast({ description: t("jira:watcherDeleted"), variant: "success" });
      } catch (err) {
        toast({ description: t("jira:deleteFailed", { error: String(err) }), variant: "error" });
      }
    },
    [remove, t, toast],
  );

  const wrappedTrigger = useCallback(
    async (w: JiraIssueWatch) => {
      try {
        const res = await trigger(w.id, w.workspaceId);
        const n = res?.newIssues ?? 0;
        // "ticket(s)" was an English plural hack: the count now selects a form,
        // so a language with more than two can express them all.
        const description =
          n > 0 ? t("jira:foundNewTickets", { count: n }) : t("jira:noNewTicketsMatched");
        toast({ description, variant: "success" });
      } catch (err) {
        toast({ description: t("jira:checkFailed", { error: String(err) }), variant: "error" });
      }
    },
    [trigger, t, toast],
  );

  const wrappedReset = useCallback(
    async (w: JiraIssueWatch) => {
      try {
        const res = await reset(w.id, w.workspaceId);
        const n = res?.tasksDeleted ?? 0;
        toast({
          description:
            n > 0
              ? t("jira:resetCompleteDeletedTasks", { count: n })
              : t("jira:resetCompleteNoTasksDeleted"),
          variant: "success",
        });
      } catch (err) {
        toast({ description: t("jira:resetFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [reset, t, toast],
  );

  return {
    create: wrappedCreate,
    update: wrappedUpdate,
    remove: wrappedDelete,
    trigger: wrappedTrigger,
    reset: wrappedReset,
  };
}

function useEnabledDrafts(items: JiraIssueWatch[], update: RawActions["update"]) {
  const saveEnabled = useCallback(
    async (watch: JiraIssueWatch, enabled: boolean) => {
      await update(watch.id, { enabled }, watch.workspaceId);
    },
    [update],
  );
  return useWatcherEnabledDrafts({ id: "jira-watch-enabled", items, saveEnabled });
}

export function JiraIssueWatchersSection() {
  const { t } = useTranslation();
  // Pass undefined to fetch every watch across every workspace.
  const { items, loading, create, update, remove, trigger, previewReset, reset } =
    useJiraIssueWatches();
  const actions = useToastedActions({ create, update, remove, trigger, reset });
  const enabledDrafts = useEnabledDrafts(items, update);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<JiraIssueWatch | null>(null);
  // resetCtrl owns the watch awaiting reset confirmation and the stable
  // preview/confirm callbacks for ResetWatchDialog. The whole row is kept
  // (not just the id) so workspaceId stays available even if the list
  // re-renders mid-dialog.
  const resetCtrl = useWatchResetController<JiraIssueWatch>({
    preview: (w) => previewReset(w.id, w.workspaceId),
    reset: (w) => actions.reset(w).then(() => undefined),
  });

  const openCreate = useCallback(() => {
    setEditing(null);
    setDialogOpen(true);
  }, []);
  const openEdit = useCallback((w: JiraIssueWatch) => {
    setEditing(w);
    setDialogOpen(true);
  }, []);

  // Adapt the watch-aware actions back to id-keyed callbacks the table expects;
  // the table looks up the watch by id when it needs to forward the per-row
  // workspaceId to mutations.
  const handleDelete = useCallback(
    (id: string) => {
      const w = items.find((item) => item.id === id);
      if (w) actions.remove(w);
    },
    [items, actions],
  );
  const handleTrigger = useCallback(
    (id: string) => {
      const w = items.find((item) => item.id === id);
      if (w) actions.trigger(w);
    },
    [items, actions],
  );
  const { setResetting } = resetCtrl;
  const handleReset = useCallback(
    (id: string) => {
      const w = items.find((item) => item.id === id);
      if (w) setResetting(w);
    },
    [items, setResetting],
  );

  return (
    <SettingsSection
      icon={<IconBellRinging className="h-5 w-5" />}
      title={t("jira:jiraWatchers")}
      description={t("jira:jiraWatchersDescription")}
      action={
        <Button size="sm" onClick={openCreate} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          {t("jira:newWatcher")}
        </Button>
      }
    >
      <WatcherSettingsCard
        isDirty={enabledDrafts.dirtyIds.size > 0}
        isLoading={loading}
        isEmpty={items.length === 0}
        testId="jira-watchers-card"
      >
        <JiraIssueWatchTable
          watches={enabledDrafts.items}
          dirtyIds={enabledDrafts.dirtyIds}
          showWorkspace
          onEdit={openEdit}
          onDelete={handleDelete}
          onTrigger={handleTrigger}
          onReset={handleReset}
          onToggleEnabled={enabledDrafts.toggleEnabled}
        />
      </WatcherSettingsCard>
      <JiraIssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editing}
        // No workspaceId — dialog shows a workspace picker for new watches and
        // pulls workspaceId from the watch row when editing.
        onCreate={actions.create}
        onUpdate={(id, req) => {
          const w = editing;
          // Unreachable-invariant guard: onUpdate only fires while a row is
          // being edited. A developer diagnostic, not user copy.
          // eslint-disable-next-line i18next/no-literal-string -- invariant message
          if (!w) throw new Error("update without editing watch");
          return actions.update(id, req, w.workspaceId);
        }}
      />
      {resetCtrl.resetting && (
        <ResetWatchDialog
          open
          onOpenChange={resetCtrl.onOpenChange}
          integrationLabel={t("jira:jiraWatcher")}
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </SettingsSection>
  );
}
