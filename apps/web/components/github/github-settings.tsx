"use client";

import { useState, useCallback } from "react";
import { IconBrandGithub, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { GitHubStatusCard } from "./github-status";
import { ReviewWatchTable } from "./review-watch-table";
import { ReviewWatchDialog } from "./review-watch-dialog";
import { IssueWatchTable } from "./issue-watch-table";
import { IssueWatchDialog } from "./issue-watch-dialog";
import { ActionPresetsSection } from "./action-presets-section";
import { DefaultQueriesSection } from "./default-queries-section";
import { PRStatsPanel } from "./pr-stats";
import { useReviewWatches } from "@/hooks/domains/github/use-review-watches";
import { useIssueWatches } from "@/hooks/domains/github/use-issue-watches";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import type { ReviewWatch, IssueWatch } from "@/lib/types/github";

function useWatchActions(workspaceId?: string | null) {
  const { items: watches, create, update, remove, trigger } = useReviewWatches(workspaceId);
  const { toast } = useToast();

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await remove(id);
        toast({ description: "Review watch deleted", variant: "success" });
      } catch {
        toast({ description: "Failed to delete review watch", variant: "error" });
      }
    },
    [remove, toast],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      try {
        const result = await trigger(id);
        const count = result?.new_prs_found ?? 0;
        if (count > 0) {
          toast({
            description: `Found ${count} new PR${count > 1 ? "s" : ""}`,
            variant: "success",
          });
        } else {
          toast({ description: "No new PRs found" });
        }
      } catch {
        toast({ description: "Failed to check for PRs", variant: "error" });
      }
    },
    [trigger, toast],
  );

  const handleToggleEnabled = useCallback(
    async (watch: ReviewWatch) => {
      try {
        await update(watch.id, { enabled: !watch.enabled });
        toast({
          description: watch.enabled ? "Watch paused" : "Watch enabled",
          variant: "success",
        });
      } catch {
        toast({ description: "Failed to update watch", variant: "error" });
      }
    },
    [update, toast],
  );

  return { watches, create, update, handleDelete, handleTrigger, handleToggleEnabled };
}

function useIssueWatchActions(workspaceId?: string | null) {
  const { items: watches, create, update, remove, trigger } = useIssueWatches(workspaceId);
  const { toast } = useToast();

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await remove(id);
        toast({ description: "Issue watch deleted", variant: "success" });
      } catch {
        toast({ description: "Failed to delete issue watch", variant: "error" });
      }
    },
    [remove, toast],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      try {
        const result = await trigger(id);
        const count = result?.new_issues_found ?? 0;
        if (count > 0) {
          toast({
            description: `Found ${count} new issue${count > 1 ? "s" : ""}`,
            variant: "success",
          });
        } else {
          toast({ description: "No new issues found" });
        }
      } catch {
        toast({ description: "Failed to check for issues", variant: "error" });
      }
    },
    [trigger, toast],
  );

  const handleToggleEnabled = useCallback(
    async (watch: IssueWatch) => {
      try {
        await update(watch.id, { enabled: !watch.enabled });
        toast({
          description: watch.enabled ? "Watch paused" : "Watch enabled",
          variant: "success",
        });
      } catch {
        toast({ description: "Failed to update watch", variant: "error" });
      }
    },
    [update, toast],
  );

  return { watches, create, update, handleDelete, handleTrigger, handleToggleEnabled };
}

export function GitHubConnectionSection() {
  return (
    <>
      <div>
        <h2 className="text-2xl font-bold flex items-center gap-2">
          <IconBrandGithub className="h-6 w-6" />
          GitHub Integration
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Connect Kandev to GitHub. Authentication is shared across all workspaces; PR/issue
          watchers and presets are configured per workspace.
        </p>
      </div>
      <Separator />
      <SettingsSection title="Connection Status" description="GitHub authentication status">
        <Card>
          <CardContent className="py-3">
            <GitHubStatusCard />
          </CardContent>
        </Card>
      </SettingsSection>
    </>
  );
}

// PerWorkspaceSection wraps the still-per-workspace surfaces (action presets,
// PR analytics) under a workspace switcher. Watchers don't go in here — they
// list flat across every workspace via their own sections.
function PerWorkspaceSection({ workspaceId }: { workspaceId: string }) {
  return (
    <div className="space-y-8">
      <ActionPresetsSection workspaceId={workspaceId} />
      <SettingsSection title="PR Analytics" description="Pull request activity for this workspace.">
        <PRStatsPanel workspaceId={workspaceId} />
      </SettingsSection>
    </div>
  );
}

export function GitHubIntegrationPage() {
  return (
    <TooltipProvider>
      <div className="space-y-8">
        <GitHubConnectionSection />
        <ReviewWatchSection />
        <IssueWatchSection />
        <WorkspaceScopedSection label="Presets and analytics for">
          {(ws) => <PerWorkspaceSection key={ws} workspaceId={ws} />}
        </WorkspaceScopedSection>
        <DefaultQueriesSection />
      </div>
    </TooltipProvider>
  );
}

// ReviewWatchSection lists every review watch across every workspace in a
// single flat table. Pass `undefined` to the hook so it fetches all rows; the
// dialog's create flow asks the user to pick the workspace.
function ReviewWatchSection() {
  const { watches, create, update, handleDelete, handleTrigger, handleToggleEnabled } =
    useWatchActions(undefined);
  const { toast } = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingWatch] = useState<ReviewWatch | null>(null);

  const handleEdit = useCallback((watch: ReviewWatch) => {
    setEditingWatch(watch);
    setDialogOpen(true);
  }, []);

  return (
    <>
      <SettingsSection
        title="Review Watches"
        description="Automatically create tasks for PRs that need your review."
        action={
          <Button
            size="sm"
            onClick={() => {
              setEditingWatch(null);
              setDialogOpen(true);
            }}
            className="cursor-pointer"
          >
            <IconPlus className="h-4 w-4 mr-1" />
            Add Watch
          </Button>
        }
      >
        <Card>
          <CardContent className="p-0">
            <ReviewWatchTable
              watches={watches}
              showWorkspace
              onEdit={handleEdit}
              onDelete={handleDelete}
              onTrigger={handleTrigger}
              onToggleEnabled={handleToggleEnabled}
            />
          </CardContent>
        </Card>
      </SettingsSection>
      <ReviewWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editingWatch}
        // No workspaceId — dialog shows a workspace picker for new watches.
        onCreate={async (req) => {
          await create(req);
          toast({ description: "Review watch created", variant: "success" });
        }}
        onUpdate={async (id, req) => {
          await update(id, req);
          toast({ description: "Review watch updated", variant: "success" });
        }}
      />
    </>
  );
}

function IssueWatchSection() {
  const issueActions = useIssueWatchActions(undefined);
  const { toast } = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingIssueWatch] = useState<IssueWatch | null>(null);

  const handleEdit = useCallback((watch: IssueWatch) => {
    setEditingIssueWatch(watch);
    setDialogOpen(true);
  }, []);

  return (
    <>
      <SettingsSection
        title="Issue Watches"
        description="Automatically create tasks for GitHub issues matching your criteria."
        action={
          <Button
            size="sm"
            onClick={() => {
              setEditingIssueWatch(null);
              setDialogOpen(true);
            }}
            className="cursor-pointer"
          >
            <IconPlus className="h-4 w-4 mr-1" />
            Add Watch
          </Button>
        }
      >
        <Card>
          <CardContent className="p-0">
            <IssueWatchTable
              watches={issueActions.watches}
              showWorkspace
              onEdit={handleEdit}
              onDelete={issueActions.handleDelete}
              onTrigger={issueActions.handleTrigger}
              onToggleEnabled={issueActions.handleToggleEnabled}
            />
          </CardContent>
        </Card>
      </SettingsSection>
      <IssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editingWatch}
        onCreate={async (req) => {
          await issueActions.create(req);
          toast({ description: "Issue watch created", variant: "success" });
        }}
        onUpdate={async (id, req) => {
          await issueActions.update(id, req);
          toast({ description: "Issue watch updated", variant: "success" });
        }}
      />
    </>
  );
}
