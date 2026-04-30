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

type Props = { workspaceId: string };

// useToastedActions wraps the raw create/update/delete/trigger callbacks with
// success/failure toasts. Kept separate so the section component stays under
// the 100-line lint limit and the toast wiring is testable in isolation.
function useToastedActions(workspaceId: string) {
  const { toast } = useToast();
  const { create, update, remove, trigger } = useJiraIssueWatches(workspaceId);

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
    async (id: string, req: Parameters<typeof update>[1]) => {
      try {
        await update(id, req);
        toast({ description: "Watcher updated", variant: "success" });
      } catch (err) {
        toast({ description: `Update failed: ${String(err)}`, variant: "error" });
        throw err;
      }
    },
    [update, toast],
  );

  const wrappedDelete = useCallback(
    async (id: string) => {
      if (!confirm("Delete this JIRA watcher?")) return;
      try {
        await remove(id);
        toast({ description: "Watcher deleted", variant: "success" });
      } catch (err) {
        toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
      }
    },
    [remove, toast],
  );

  const wrappedTrigger = useCallback(
    async (id: string) => {
      try {
        const res = await trigger(id);
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
        await update(w.id, { enabled: !w.enabled });
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

/**
 * JiraIssueWatchersSection renders the "JIRA watchers" block on the JIRA
 * settings page: a table of configured watchers, a "+ New" button, and a
 * dialog for create/edit. Mirrors the GitHub issue-watch UI patterns.
 */
export function JiraIssueWatchersSection({ workspaceId }: Props) {
  const { items, loading } = useJiraIssueWatches(workspaceId);
  const actions = useToastedActions(workspaceId);

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
              onEdit={openEdit}
              onDelete={actions.remove}
              onTrigger={actions.trigger}
              onToggleEnabled={actions.toggleEnabled}
            />
          )}
        </CardContent>
      </Card>
      <JiraIssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editing}
        workspaceId={workspaceId}
        onCreate={actions.create}
        onUpdate={actions.update}
      />
    </SettingsSection>
  );
}
