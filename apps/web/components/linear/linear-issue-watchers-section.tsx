"use client";

import { useCallback, useState } from "react";
import { IconBellRinging, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useToast } from "@/components/toast-provider";
import { useLinearIssueWatches } from "@/hooks/domains/linear/use-linear-issue-watches";
import { LinearIssueWatchTable } from "./linear-issue-watch-table";
import { LinearIssueWatchDialog } from "./linear-issue-watch-dialog";
import type { LinearIssueWatch } from "@/lib/types/linear";

// LinearIssueWatchersSection lists watches across every workspace in a single
// flat table on the install-wide settings page. The dialog's create flow asks
// the user to pick the workspace; per-row mutations forward each watch's
// stored workspaceId so the backend's IDOR guard accepts them.
type RawActions = {
  create: ReturnType<typeof useLinearIssueWatches>["create"];
  update: ReturnType<typeof useLinearIssueWatches>["update"];
  remove: ReturnType<typeof useLinearIssueWatches>["remove"];
  trigger: ReturnType<typeof useLinearIssueWatches>["trigger"];
};

function useToastedActions({ create, update, remove, trigger }: RawActions) {
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
    async (id: string, req: Parameters<typeof update>[1], rowWorkspaceId: string) => {
      try {
        await update(id, req, rowWorkspaceId);
        toast({ description: "Watcher updated", variant: "success" });
      } catch (err) {
        toast({ description: `Update failed: ${String(err)}`, variant: "error" });
        throw err;
      }
    },
    [update, toast],
  );

  const wrappedDelete = useCallback(
    async (w: LinearIssueWatch) => {
      if (!confirm("Delete this Linear watcher?")) return;
      try {
        await remove(w.id, w.workspaceId);
        toast({ description: "Watcher deleted", variant: "success" });
      } catch (err) {
        toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
      }
    },
    [remove, toast],
  );

  const wrappedTrigger = useCallback(
    async (w: LinearIssueWatch) => {
      try {
        const res = await trigger(w.id, w.workspaceId);
        const n = res?.newIssues ?? 0;
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
    async (w: LinearIssueWatch) => {
      try {
        await update(w.id, { enabled: !w.enabled }, w.workspaceId);
      } catch (err) {
        toast({ description: `Toggle failed: ${String(err)}`, variant: "error" });
      }
    },
    [update, toast],
  );

  return {
    create: wrappedCreate,
    update: wrappedUpdate,
    remove: wrappedDelete,
    trigger: wrappedTrigger,
    toggleEnabled,
  };
}

export function LinearIssueWatchersSection() {
  const { items, loading, create, update, remove, trigger } = useLinearIssueWatches();
  const actions = useToastedActions({ create, update, remove, trigger });

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<LinearIssueWatch | null>(null);

  const openCreate = useCallback(() => {
    setEditing(null);
    setDialogOpen(true);
  }, []);
  const openEdit = useCallback((w: LinearIssueWatch) => {
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

  return (
    <SettingsSection
      icon={<IconBellRinging className="h-5 w-5" />}
      title="Linear watchers"
      description="Poll a Linear filter and auto-create a Kandev task for each newly-matching issue."
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
            <LinearIssueWatchTable
              watches={items}
              showWorkspace
              onEdit={openEdit}
              onDelete={handleDelete}
              onTrigger={handleTrigger}
              onToggleEnabled={actions.toggleEnabled}
            />
          )}
        </CardContent>
      </Card>
      <LinearIssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editing}
        onCreate={actions.create}
        onUpdate={(id, req) => {
          const w = editing;
          if (!w) throw new Error("update without editing watch");
          return actions.update(id, req, w.workspaceId);
        }}
      />
    </SettingsSection>
  );
}
