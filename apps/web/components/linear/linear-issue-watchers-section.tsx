"use client";

import { useCallback, useState } from "react";
import { IconBellRinging, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { SettingsSection } from "@/components/settings/settings-section";
import { WatcherSettingsCard } from "@/components/integrations/watcher-settings-card";
import { useToast } from "@/components/toast-provider";
import { useLinearIssueWatches } from "@/hooks/domains/linear/use-linear-issue-watches";
import { useWatcherEnabledDrafts } from "@/components/integrations/use-watcher-enabled-drafts";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { LinearIssueWatchTable } from "./linear-issue-watch-table";
import { LinearIssueWatchDialog } from "./linear-issue-watch-dialog";
import type { LinearIssueWatch } from "@/lib/types/linear";
import { useTranslation } from "react-i18next";

// LinearIssueWatchersSection lists watches across every workspace in a single
// flat table on the install-wide settings page. The dialog's create flow asks
// the user to pick the workspace; per-row mutations forward each watch's
// stored workspaceId so the backend's IDOR guard accepts them.
type RawActions = {
  create: ReturnType<typeof useLinearIssueWatches>["create"];
  update: ReturnType<typeof useLinearIssueWatches>["update"];
  remove: ReturnType<typeof useLinearIssueWatches>["remove"];
  trigger: ReturnType<typeof useLinearIssueWatches>["trigger"];
  reset: ReturnType<typeof useLinearIssueWatches>["reset"];
};

function useToastedActions({ create, update, remove, trigger, reset }: RawActions) {
  const { t } = useTranslation();
  const { toast } = useToast();

  const wrappedCreate = useCallback(
    async (req: Parameters<typeof create>[0]) => {
      try {
        await create(req);
        toast({ description: t("linear:watcherCreated"), variant: "success" });
      } catch (err) {
        toast({ description: t("linear:createFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [create, t, toast],
  );

  const wrappedUpdate = useCallback(
    async (id: string, req: Parameters<typeof update>[1], rowWorkspaceId: string) => {
      try {
        await update(id, req, rowWorkspaceId);
        toast({ description: t("linear:watcherUpdated"), variant: "success" });
      } catch (err) {
        toast({ description: t("linear:updateFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [update, t, toast],
  );

  const wrappedDelete = useCallback(
    async (w: LinearIssueWatch) => {
      try {
        await remove(w.id, w.workspaceId);
        toast({ description: t("linear:watcherDeleted"), variant: "success" });
      } catch (err) {
        toast({ description: t("linear:deleteFailed", { error: String(err) }), variant: "error" });
      }
    },
    [remove, t, toast],
  );

  const wrappedTrigger = useCallback(
    async (w: LinearIssueWatch) => {
      try {
        const res = await trigger(w.id, w.workspaceId);
        const n = res?.newIssues ?? 0;
        // "issue(s)" was an English plural hack: the count now selects a form,
        // so a language with more than two can express them all.
        const description =
          n > 0 ? t("linear:foundNewIssues", { count: n }) : t("linear:noNewIssuesMatched");
        toast({ description, variant: "success" });
      } catch (err) {
        toast({ description: t("linear:checkFailed", { error: String(err) }), variant: "error" });
      }
    },
    [trigger, t, toast],
  );

  const wrappedReset = useCallback(
    async (w: LinearIssueWatch) => {
      try {
        const res = await reset(w.id, w.workspaceId);
        const n = res?.tasksDeleted ?? 0;
        toast({
          description:
            n > 0
              ? t("linear:resetCompleteDeletedTasks", { count: n })
              : t("linear:resetCompleteNoTasksDeleted"),
          variant: "success",
        });
      } catch (err) {
        toast({ description: t("linear:resetFailed", { error: String(err) }), variant: "error" });
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

function useEnabledDrafts(items: LinearIssueWatch[], update: RawActions["update"]) {
  const saveEnabled = useCallback(
    async (watch: LinearIssueWatch, enabled: boolean) => {
      await update(watch.id, { enabled }, watch.workspaceId);
    },
    [update],
  );
  return useWatcherEnabledDrafts({ id: "linear-watch-enabled", items, saveEnabled });
}

export function LinearIssueWatchersSection() {
  const { t } = useTranslation();
  const { items, loading, create, update, remove, trigger, previewReset, reset } =
    useLinearIssueWatches();
  const actions = useToastedActions({ create, update, remove, trigger, reset });
  const enabledDrafts = useEnabledDrafts(items, update);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<LinearIssueWatch | null>(null);
  const resetCtrl = useWatchResetController<LinearIssueWatch>({
    preview: (w) => previewReset(w.id, w.workspaceId),
    reset: (w) => actions.reset(w).then(() => undefined),
  });

  const openCreate = useCallback(() => {
    setEditing(null);
    setDialogOpen(true);
  }, []);
  const openEdit = useCallback(
    (w: LinearIssueWatch) => {
      setEditing(items.find((item) => item.id === w.id) ?? w);
      setDialogOpen(true);
    },
    [items],
  );

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
      title={t("linear:linearWatchers")}
      description={t("linear:linearWatchersDescription")}
      action={
        <Button size="sm" onClick={openCreate} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          {t("linear:newWatcher")}
        </Button>
      }
    >
      <WatcherSettingsCard
        isDirty={enabledDrafts.dirtyIds.size > 0}
        isLoading={loading}
        isEmpty={items.length === 0}
        testId="linear-watchers-card"
      >
        <LinearIssueWatchTable
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
      <LinearIssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editing}
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
          integrationLabel={t("linear:linearWatcher")}
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </SettingsSection>
  );
}
