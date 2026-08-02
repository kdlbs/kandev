"use client";

import { useCallback, useState } from "react";
import { IconBellRinging, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { SettingsSection } from "@/components/settings/settings-section";
import { WatcherSettingsCard } from "@/components/integrations/watcher-settings-card";
import { useToast } from "@/components/toast-provider";
import { useSentryIssueWatches } from "@/hooks/domains/sentry/use-sentry-issue-watches";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import { useWatcherEnabledDrafts } from "@/components/integrations/use-watcher-enabled-drafts";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { SentryIssueWatchTable } from "./sentry-issue-watch-table";
import { SentryIssueWatchDialog } from "./sentry-issue-watch-dialog";
import type { SentryIssueWatch } from "@/lib/types/sentry";
import { useTranslation } from "react-i18next";

// SentryIssueWatchersSection lists watches for the workspace resolved by its
// parent integration page. Creating a watch is locked to that workspace and
// requires picking one of its Sentry instances.
type RawActions = {
  create: ReturnType<typeof useSentryIssueWatches>["create"];
  update: ReturnType<typeof useSentryIssueWatches>["update"];
  remove: ReturnType<typeof useSentryIssueWatches>["remove"];
  trigger: ReturnType<typeof useSentryIssueWatches>["trigger"];
  reset: ReturnType<typeof useSentryIssueWatches>["reset"];
};

function useToastedActions({ create, update, remove, trigger, reset }: RawActions) {
  const { t } = useTranslation();
  const { toast } = useToast();

  const wrappedCreate = useCallback(
    async (req: Parameters<typeof create>[0]) => {
      try {
        await create(req);
        toast({ description: t("sentry:watcherCreated"), variant: "success" });
      } catch (err) {
        toast({ description: t("sentry:createFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [create, toast, t],
  );

  const wrappedUpdate = useCallback(
    async (id: string, workspaceId: string, req: Parameters<typeof update>[2]) => {
      try {
        await update(id, workspaceId, req);
        toast({ description: t("sentry:watcherUpdated"), variant: "success" });
      } catch (err) {
        toast({ description: t("sentry:updateFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [update, toast, t],
  );

  const wrappedDelete = useCallback(
    async (id: string, workspaceId: string) => {
      if (!confirm(t("sentry:deleteThisSentryWatcher"))) return;
      try {
        await remove(id, workspaceId);
        toast({ description: t("sentry:watcherDeleted"), variant: "success" });
      } catch (err) {
        toast({ description: t("sentry:deleteFailed", { error: String(err) }), variant: "error" });
      }
    },
    [remove, toast, t],
  );

  const wrappedTrigger = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const res = await trigger(id, workspaceId);
        const n = res?.published ?? 0;
        // The old copy hedged with "issue(s)". `count` lets i18next pick the
        // form, which reads better in English and is the only shape a language
        // with more than two plural forms can translate.
        const description =
          n > 0 ? t("sentry:foundNewIssues", { count: n }) : t("sentry:noNewIssuesMatched");
        toast({ description, variant: "success" });
      } catch (err) {
        toast({ description: t("sentry:checkFailed", { error: String(err) }), variant: "error" });
      }
    },
    [trigger, toast, t],
  );

  const wrappedReset = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const res = await reset(id, workspaceId);
        const n = res?.tasksDeleted ?? 0;
        toast({
          description:
            n > 0
              ? t("sentry:resetCompleteDeletedTasks", { count: n })
              : t("sentry:resetCompleteNoTasksDeleted"),
          variant: "success",
        });
      } catch (err) {
        toast({ description: t("sentry:resetFailed", { error: String(err) }), variant: "error" });
        throw err;
      }
    },
    [reset, toast, t],
  );

  return {
    create: wrappedCreate,
    update: wrappedUpdate,
    remove: wrappedDelete,
    trigger: wrappedTrigger,
    reset: wrappedReset,
  };
}

export function SentryIssueWatchersSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { items, loading, create, update, remove, trigger, previewReset, reset } =
    useSentryIssueWatches(workspaceId);
  const { instances } = useSentryInstances(workspaceId);
  const instanceName = useCallback(
    (id: string) => {
      // An em dash is a glyph, not copy — it stays out of the catalog. The
      // resolved name is the user's own instance name, also never translated.
      if (!id) return "—";
      return instances.find((i) => i.id === id)?.name ?? t("sentry:instanceUnavailable");
    },
    [instances, t],
  );
  const actions = useToastedActions({ create, update, remove, trigger, reset });
  const saveEnabled = useCallback(
    async (watch: SentryIssueWatch, enabled: boolean) => {
      await update(watch.id, watch.workspaceId, { enabled });
    },
    [update],
  );
  const enabledDrafts = useWatcherEnabledDrafts({
    id: `sentry-watch-enabled:${workspaceId}`,
    items,
    saveEnabled,
  });

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<SentryIssueWatch | null>(null);
  const resetCtrl = useWatchResetController<SentryIssueWatch>({
    preview: (w) => previewReset(w.id, w.workspaceId),
    reset: (w) => actions.reset(w.id, w.workspaceId).then(() => undefined),
  });

  const openCreate = useCallback(() => {
    setEditing(null);
    setDialogOpen(true);
  }, []);
  const openEdit = useCallback((w: SentryIssueWatch) => {
    setEditing(w);
    setDialogOpen(true);
  }, []);

  const { setResetting } = resetCtrl;
  const handleReset = useCallback(
    (id: string, _workspaceId: string) => {
      const w = items.find((item) => item.id === id);
      if (w) setResetting(w);
    },
    [items, setResetting],
  );

  return (
    <SettingsSection
      icon={<IconBellRinging className="h-5 w-5" />}
      title={t("sentry:sentryWatchers")}
      description={t("sentry:sentryWatchersDescription")}
      action={
        <Button size="sm" onClick={openCreate} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          {t("sentry:newWatcher")}
        </Button>
      }
    >
      <WatcherSettingsCard
        isDirty={enabledDrafts.dirtyIds.size > 0}
        isLoading={loading}
        isEmpty={items.length === 0}
        testId="sentry-watchers-card"
      >
        <SentryIssueWatchTable
          watches={enabledDrafts.items}
          dirtyIds={enabledDrafts.dirtyIds}
          instanceName={instanceName}
          onEdit={openEdit}
          onDelete={actions.remove}
          onTrigger={actions.trigger}
          onReset={handleReset}
          onToggleEnabled={enabledDrafts.toggleEnabled}
        />
      </WatcherSettingsCard>
      <SentryIssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editing}
        onCreate={actions.create}
        onUpdate={actions.update}
      />
      {resetCtrl.resetting && (
        <ResetWatchDialog
          open
          onOpenChange={resetCtrl.onOpenChange}
          integrationLabel={t("sentry:sentryWatcher")}
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </SettingsSection>
  );
}
