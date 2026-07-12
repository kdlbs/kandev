"use client";

import { useState, useCallback } from "react";
import { IconBrandGithub, IconPlus, IconTrashX } from "@tabler/icons-react";
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
import { GitHubRepoScopeSection } from "./github-repo-scope-section";
import { PRStatsPanel } from "./pr-stats";
import { useReviewWatches } from "@/hooks/domains/github/use-review-watches";
import { useIssueWatches } from "@/hooks/domains/github/use-issue-watches";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { cleanupMergedReviewTasks, cleanupClosedIssueTasks } from "@/lib/api/domains/github-api";
import type { ReviewWatch, IssueWatch } from "@/lib/types/github";

// CleanupNowButton runs a manual global sweep over the dedup tables. Useful
// for users who upgraded with a pile of legacy merged-PR / closed-issue
// review tasks that pre-date the cleanup_policy work — clicking once drains
// them according to each watch's policy.
function CleanupNowButton({
  label,
  run,
}: {
  label: string;
  run: () => Promise<{ deleted: number }>;
}) {
  const { toast } = useToast();
  const [busy, setBusy] = useState(false);
  return (
    <Button
      size="sm"
      variant="outline"
      disabled={busy}
      onClick={async () => {
        setBusy(true);
        try {
          const { deleted } = await run();
          toast({
            description:
              deleted === 0
                ? "No tasks to clean up"
                : `Deleted ${deleted} task${deleted === 1 ? "" : "s"}`,
            variant: "success",
          });
        } catch {
          toast({ description: "Cleanup failed", variant: "error" });
        } finally {
          setBusy(false);
        }
      }}
      className="cursor-pointer"
    >
      <IconTrashX className="h-4 w-4 mr-1" />
      {busy ? "Cleaning…" : label}
    </Button>
  );
}

function useWatchActions(workspaceId?: string | null) {
  const {
    items: watches,
    create,
    update,
    remove,
    trigger,
    previewReset,
    reset,
  } = useReviewWatches(workspaceId);
  const { toast } = useToast();

  const handleDelete = useCallback(
    async (id: string) => {
      const watch = watches.find((item) => item.id === id);
      if (!watch) {
        toast({ description: "Review watch not found", variant: "error" });
        return;
      }
      try {
        await remove(id, watch.workspace_id);
        toast({ description: "Review watch deleted", variant: "success" });
      } catch {
        toast({ description: "Failed to delete review watch", variant: "error" });
      }
    },
    [remove, toast, watches],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      const watch = watches.find((item) => item.id === id);
      if (!watch) {
        toast({ description: "Review watch not found", variant: "error" });
        return;
      }
      try {
        const result = await trigger(id, watch.workspace_id);
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
    [trigger, toast, watches],
  );

  const handleToggleEnabled = useCallback(
    async (watch: ReviewWatch) => {
      try {
        await update(watch.id, watch.workspace_id, { enabled: !watch.enabled });
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

  const handleReset = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const { tasksDeleted } = await reset(id, workspaceId);
        toast({
          description:
            tasksDeleted > 0
              ? `Reset complete — deleted ${tasksDeleted} task(s); next poll will re-import.`
              : "Reset complete — next poll will re-import matches.",
          variant: "success",
        });
      } catch {
        toast({ description: "Failed to reset review watch", variant: "error" });
        throw new Error("reset failed");
      }
    },
    [reset, toast],
  );

  return {
    watches,
    create,
    update,
    previewReset,
    handleDelete,
    handleTrigger,
    handleReset,
    handleToggleEnabled,
  };
}

function useIssueWatchActions(workspaceId?: string | null) {
  const {
    items: watches,
    create,
    update,
    remove,
    trigger,
    previewReset,
    reset,
  } = useIssueWatches(workspaceId);
  const { toast } = useToast();

  const handleDelete = useCallback(
    async (id: string) => {
      const watch = watches.find((item) => item.id === id);
      if (!watch) {
        toast({ description: "Issue watch not found", variant: "error" });
        return;
      }
      try {
        await remove(id, watch.workspace_id);
        toast({ description: "Issue watch deleted", variant: "success" });
      } catch {
        toast({ description: "Failed to delete issue watch", variant: "error" });
      }
    },
    [remove, toast, watches],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      const watch = watches.find((item) => item.id === id);
      if (!watch) {
        toast({ description: "Issue watch not found", variant: "error" });
        return;
      }
      try {
        const result = await trigger(id, watch.workspace_id);
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
    [trigger, toast, watches],
  );

  const handleToggleEnabled = useCallback(
    async (watch: IssueWatch) => {
      try {
        await update(watch.id, watch.workspace_id, { enabled: !watch.enabled });
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

  const handleReset = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const { tasksDeleted } = await reset(id, workspaceId);
        toast({
          description:
            tasksDeleted > 0
              ? `Reset complete — deleted ${tasksDeleted} task(s); next poll will re-import.`
              : "Reset complete — next poll will re-import matches.",
          variant: "success",
        });
      } catch {
        toast({ description: "Failed to reset issue watch", variant: "error" });
        throw new Error("reset failed");
      }
    },
    [reset, toast],
  );

  return {
    watches,
    create,
    update,
    previewReset,
    handleDelete,
    handleTrigger,
    handleReset,
    handleToggleEnabled,
  };
}

export function GitHubConnectionSection() {
  return (
    <>
      <div>
        <h2
          className="text-2xl font-bold flex items-center gap-2"
          data-testid="github-integration-heading"
        >
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

function PerWorkspaceSection({ workspaceId }: { workspaceId: string }) {
  return (
    <div className="space-y-8">
      <GitHubRepoScopeSection workspaceId={workspaceId} />
      <ReviewWatchSection workspaceId={workspaceId} />
      <IssueWatchSection workspaceId={workspaceId} />
      <ActionPresetsSection workspaceId={workspaceId} />
      <SettingsSection title="PR Analytics" description="Pull request activity for this workspace.">
        <PRStatsPanel workspaceId={workspaceId} />
      </SettingsSection>
      <DefaultQueriesSection workspaceId={workspaceId} />
    </div>
  );
}

type GitHubIntegrationPageProps = {
  workspaceId?: string;
};

export function GitHubIntegrationPage({ workspaceId }: GitHubIntegrationPageProps = {}) {
  return (
    <TooltipProvider>
      <div className="space-y-8">
        <GitHubConnectionSection />
        <WorkspaceScopedSection workspaceId={workspaceId}>
          {(ws) => <PerWorkspaceSection key={ws} workspaceId={ws} />}
        </WorkspaceScopedSection>
      </div>
    </TooltipProvider>
  );
}

function ReviewWatchSection({ workspaceId }: { workspaceId: string }) {
  const {
    watches,
    create,
    update,
    previewReset,
    handleDelete,
    handleTrigger,
    handleReset,
    handleToggleEnabled,
  } = useWatchActions(workspaceId);
  const { toast } = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingWatch] = useState<ReviewWatch | null>(null);
  const resetCtrl = useWatchResetController<ReviewWatch>({
    preview: (w) => previewReset(w.id, w.workspace_id),
    reset: (w) => handleReset(w.id, w.workspace_id),
  });

  const handleEdit = useCallback((watch: ReviewWatch) => {
    setEditingWatch(watch);
    setDialogOpen(true);
  }, []);

  const { setResetting: setReviewResetting } = resetCtrl;
  const onResetClick = useCallback(
    (id: string) => {
      const w = watches.find((item) => item.id === id);
      if (w) setReviewResetting(w);
    },
    [watches, setReviewResetting],
  );

  return (
    <>
      <SettingsSection
        title="Review Watches"
        description="Automatically create tasks for PRs that need your review."
        action={
          <div className="flex items-center gap-2">
            <CleanupNowButton
              label="Clean up merged"
              run={() => cleanupMergedReviewTasks(workspaceId)}
            />
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
          </div>
        }
      >
        <Card>
          <CardContent className="p-0">
            <ReviewWatchTable
              watches={watches}
              onEdit={handleEdit}
              onDelete={handleDelete}
              onTrigger={handleTrigger}
              onReset={onResetClick}
              onToggleEnabled={handleToggleEnabled}
            />
          </CardContent>
        </Card>
      </SettingsSection>
      <ReviewWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editingWatch}
        workspaceId={workspaceId}
        onCreate={async (req) => {
          await create(req);
          toast({ description: "Review watch created", variant: "success" });
        }}
        onUpdate={async (id, req) => {
          const watch = watches.find((item) => item.id === id);
          if (!watch) throw new Error("review watch not found");
          await update(id, watch.workspace_id, req);
          toast({ description: "Review watch updated", variant: "success" });
        }}
      />
      {resetCtrl.resetting && (
        <ResetWatchDialog
          open
          onOpenChange={resetCtrl.onOpenChange}
          integrationLabel="review watch"
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </>
  );
}

function IssueWatchSection({ workspaceId }: { workspaceId: string }) {
  const issueActions = useIssueWatchActions(workspaceId);
  const { toast } = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingIssueWatch] = useState<IssueWatch | null>(null);
  const resetCtrl = useWatchResetController<IssueWatch>({
    preview: (w) => issueActions.previewReset(w.id, w.workspace_id),
    reset: (w) => issueActions.handleReset(w.id, w.workspace_id),
  });

  const handleEdit = useCallback((watch: IssueWatch) => {
    setEditingIssueWatch(watch);
    setDialogOpen(true);
  }, []);

  const { setResetting: setIssueResetting } = resetCtrl;
  const onResetClick = useCallback(
    (id: string) => {
      const w = issueActions.watches.find((item) => item.id === id);
      if (w) setIssueResetting(w);
    },
    [issueActions.watches, setIssueResetting],
  );

  return (
    <>
      <SettingsSection
        title="Issue Watches"
        description="Automatically create tasks for GitHub issues matching your criteria."
        action={
          <div className="flex items-center gap-2">
            <CleanupNowButton
              label="Clean up closed"
              run={() => cleanupClosedIssueTasks(workspaceId)}
            />
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
          </div>
        }
      >
        <Card>
          <CardContent className="p-0">
            <IssueWatchTable
              watches={issueActions.watches}
              onEdit={handleEdit}
              onDelete={issueActions.handleDelete}
              onTrigger={issueActions.handleTrigger}
              onReset={onResetClick}
              onToggleEnabled={issueActions.handleToggleEnabled}
            />
          </CardContent>
        </Card>
      </SettingsSection>
      <IssueWatchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        watch={editingWatch}
        workspaceId={workspaceId}
        onCreate={async (req) => {
          await issueActions.create(req);
          toast({ description: "Issue watch created", variant: "success" });
        }}
        onUpdate={async (id, req) => {
          const watch = issueActions.watches.find((item) => item.id === id);
          if (!watch) throw new Error("issue watch not found");
          await issueActions.update(id, watch.workspace_id, req);
          toast({ description: "Issue watch updated", variant: "success" });
        }}
      />
      {resetCtrl.resetting && (
        <ResetWatchDialog
          open
          onOpenChange={resetCtrl.onOpenChange}
          integrationLabel="issue watch"
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </>
  );
}
