"use client";

import { useCallback, useState } from "react";
import { IconBellRinging, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useToast } from "@/components/toast-provider";
import { useSentryIssueWatches } from "@/hooks/domains/sentry/use-sentry-issue-watches";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { SentryIssueWatchTable } from "./sentry-issue-watch-table";
import { SentryIssueWatchDialog } from "./sentry-issue-watch-dialog";
import type { SentryIssueWatch } from "@/lib/types/sentry";

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
  const { toast } = useToast();

  const wrappedCreate = useCallback(
    async (req: Parameters<typeof create>[0]) => {
      try {
        await create(req);
        toast({ description: "Watcher created", variant: "success" });
      } catch (err) {
        toast({ description: `Create failed: ${String(err)}`, variant: "error" });
        throw err;
      }
    },
    [create, toast],
  );

  const wrappedUpdate = useCallback(
    async (id: string, workspaceId: string, req: Parameters<typeof update>[2]) => {
      try {
        await update(id, workspaceId, req);
        toast({ description: "Watcher updated", variant: "success" });
      } catch (err) {
        toast({ description: `Update failed: ${String(err)}`, variant: "error" });
        throw err;
      }
    },
    [update, toast],
  );

  const wrappedDelete = useCallback(
    async (id: string, workspaceId: string) => {
      if (!confirm("Delete this Sentry watcher?")) return;
      try {
        await remove(id, workspaceId);
        toast({ description: "Watcher deleted", variant: "success" });
      } catch (err) {
        toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
      }
    },
    [remove, toast],
  );

  const wrappedTrigger = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const res = await trigger(id, workspaceId);
        const n = res?.published ?? 0;
        const description =
          n > 0 ? `Found ${n} new issue(s) — tasks will appear shortly.` : "No new issues matched.";
        toast({ description, variant: "success" });
      } catch (err) {
        toast({ description: `Check failed: ${String(err)}`, variant: "error" });
      }
    },
    [trigger, toast],
  );

  const toggleEnabled = useCallback(
    async (w: SentryIssueWatch) => {
      try {
        await update(w.id, w.workspaceId, { enabled: !w.enabled });
      } catch (err) {
        toast({ description: `Toggle failed: ${String(err)}`, variant: "error" });
      }
    },
    [update, toast],
  );

  const wrappedReset = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const res = await reset(id, workspaceId);
        const n = res?.tasksDeleted ?? 0;
        toast({
          description:
            n > 0
              ? `Reset complete — deleted ${n} task(s); next poll will re-import matches.`
              : "Reset complete — next poll will re-import matches.",
          variant: "success",
        });
      } catch (err) {
        toast({ description: `Reset failed: ${String(err)}`, variant: "error" });
        throw err;
      }
    },
    [reset, toast],
  );

  return {
    create: wrappedCreate,
    update: wrappedUpdate,
    remove: wrappedDelete,
    trigger: wrappedTrigger,
    reset: wrappedReset,
    toggleEnabled,
  };
}

export function SentryIssueWatchersSection({ workspaceId }: { workspaceId: string }) {
  const { items, loading, create, update, remove, trigger, previewReset, reset } =
    useSentryIssueWatches(workspaceId);
  const { instances } = useSentryInstances(workspaceId);
  const instanceName = useCallback(
    (id: string) => {
      if (!id) return "—";
      return instances.find((i) => i.id === id)?.name ?? "(unavailable)";
    },
    [instances],
  );
  const actions = useToastedActions({ create, update, remove, trigger, reset });

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
      title="Sentry watchers"
      description="Poll a Sentry filter and auto-create a Kandev task for each newly-matching issue."
      action={
        <Button size="sm" onClick={openCreate} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          New watcher
        </Button>
      }
    >
      <Card>
        <CardContent className="pt-6">
          {loading && items.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">Loading…</p>
          ) : (
            <SentryIssueWatchTable
              watches={items}
              instanceName={instanceName}
              onEdit={openEdit}
              onDelete={actions.remove}
              onTrigger={actions.trigger}
              onReset={handleReset}
              onToggleEnabled={actions.toggleEnabled}
            />
          )}
        </CardContent>
      </Card>
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
          integrationLabel="Sentry watcher"
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </SettingsSection>
  );
}
