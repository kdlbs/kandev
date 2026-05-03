"use client";

import { useCallback, useState } from "react";
import { IconBellRinging, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useToast } from "@/components/toast-provider";
import { useJiraIssueWatches } from "@/hooks/domains/jira/use-jira-issue-watches";
import { JiraIssueWatchTable } from "./jira-issue-watch-table";
import { JiraIssueWatchDialog } from "./jira-issue-watch-dialog";
import type { JiraIssueWatch } from "@/lib/types/jira";

// JiraIssueWatchersSection lists watches across every workspace in a single
// flat table on the install-wide settings page. The dialog's create flow asks
// the user to pick the workspace; per-row mutations forward each watch's
// stored workspaceId so the backend's IDOR guard accepts them.
type RawActions = {
  create: ReturnType<typeof useJiraIssueWatches>["create"];
  update: ReturnType<typeof useJiraIssueWatches>["update"];
  remove: ReturnType<typeof useJiraIssueWatches>["remove"];
  trigger: ReturnType<typeof useJiraIssueWatches>["trigger"];
};

// useToastedActions wraps the raw create/update/delete/trigger callbacks with
// success/failure toasts. Per-row mutations need each watch's own workspaceId,
// so the wrappers take the watch (not just an id) and pass it through.
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
    async (w: JiraIssueWatch) => {
      if (!confirm("Delete this JIRA watcher?")) return;
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
    async (w: JiraIssueWatch) => {
      try {
        const res = await trigger(w.id, w.workspaceId);
        const n = res?.newIssues ?? 0;
        const description =
          n > 0
            ? `Found ${n} new ticket(s) — tasks will appear shortly.`
            : "No new tickets matched.";
        toast({ description, variant: "success" });
      } catch (err) {
        toast({ description: `Check failed: ${String(err)}`, variant: "error" });
      }
    },
    [trigger, toast],
  );

  const toggleEnabled = useCallback(
    async (w: JiraIssueWatch) => {
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

export function JiraIssueWatchersSection() {
  // Pass undefined to fetch every watch across every workspace.
  const { items, loading, create, update, remove, trigger } = useJiraIssueWatches();
  const actions = useToastedActions({ create, update, remove, trigger });

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<JiraIssueWatch | null>(null);

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

  return (
    <SettingsSection
      icon={<IconBellRinging className="h-5 w-5" />}
      title="JIRA watchers"
      description="Poll a JQL query and auto-create a Kandev task for each newly-matching ticket."
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
            <JiraIssueWatchTable
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
      <JiraIssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editing}
        // No workspaceId — dialog shows a workspace picker for new watches and
        // pulls workspaceId from the watch row when editing.
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
