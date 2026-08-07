"use client";

import { useState, useCallback } from "react";
import { IconBrandGithub, IconPlus, IconTrashX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { GitHubEnabledControl } from "@/components/github/github-enabled-control";
import { GitHubAutomationSettings, GitHubPersonalSettings } from "./github-status";
import { GitHubCallbackNotice } from "./github-callback-notice";
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
import { useWatcherEnabledDrafts } from "@/components/integrations/use-watcher-enabled-drafts";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { cleanupMergedReviewTasks, cleanupClosedIssueTasks } from "@/lib/api/domains/github-api";
import type { ReviewWatch, IssueWatch } from "@/lib/types/github";
import { useTranslation } from "react-i18next";
import { INTEGRATION_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/integrations";

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
  const { t } = useTranslation();
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
                ? t("github:noTasksToCleanUp")
                : t("github:deletedTasks", { count: deleted }),
            variant: "success",
          });
        } catch {
          toast({ description: t("github:cleanupFailed"), variant: "error" });
        } finally {
          setBusy(false);
        }
      }}
      className="cursor-pointer"
    >
      <IconTrashX className="h-4 w-4 mr-1" />
      {busy ? t("github:cleaning") : label}
    </Button>
  );
}

function useWatchActions(workspaceId?: string | null) {
  const { t } = useTranslation();
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
        toast({ description: t("github:reviewWatchNotFound"), variant: "error" });
        return;
      }
      try {
        await remove(id, watch.workspace_id);
        toast({ description: t("github:reviewWatchDeleted"), variant: "success" });
      } catch {
        toast({ description: t("github:failedToDeleteReviewWatch"), variant: "error" });
      }
    },
    [remove, t, toast, watches],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      const watch = watches.find((item) => item.id === id);
      if (!watch) {
        toast({ description: t("github:reviewWatchNotFound"), variant: "error" });
        return;
      }
      try {
        const result = await trigger(id, watch.workspace_id);
        const count = result?.new_prs_found ?? 0;
        if (count > 0) {
          toast({
            description: t("github:foundNewPrs", { count }),
            variant: "success",
          });
        } else {
          toast({ description: t("github:noNewPrsFound") });
        }
      } catch {
        toast({ description: t("github:failedToCheckForPrs"), variant: "error" });
      }
    },
    [trigger, t, toast, watches],
  );

  const handleReset = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const { tasksDeleted } = await reset(id, workspaceId);
        toast({
          description:
            tasksDeleted > 0
              ? t("github:resetCompleteDeletedTasks", { count: tasksDeleted })
              : t("github:resetCompleteNextPollWillRe"),
          variant: "success",
        });
      } catch {
        toast({ description: t("github:failedToResetReviewWatch"), variant: "error" });
        throw new Error("reset failed");
      }
    },
    [reset, t, toast],
  );

  return {
    watches,
    create,
    update,
    previewReset,
    handleDelete,
    handleTrigger,
    handleReset,
  };
}

function useIssueWatchActions(workspaceId?: string | null) {
  const { t } = useTranslation();
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
        toast({ description: t("github:issueWatchNotFound"), variant: "error" });
        return;
      }
      try {
        await remove(id, watch.workspace_id);
        toast({ description: t("github:issueWatchDeleted"), variant: "success" });
      } catch {
        toast({ description: t("github:failedToDeleteIssueWatch"), variant: "error" });
      }
    },
    [remove, t, toast, watches],
  );

  const handleTrigger = useCallback(
    async (id: string) => {
      const watch = watches.find((item) => item.id === id);
      if (!watch) {
        toast({ description: t("github:issueWatchNotFound"), variant: "error" });
        return;
      }
      try {
        const result = await trigger(id, watch.workspace_id);
        const count = result?.new_issues_found ?? 0;
        if (count > 0) {
          toast({
            description: t("github:foundNewIssues", { count }),
            variant: "success",
          });
        } else {
          toast({ description: t("github:noNewIssuesFound") });
        }
      } catch {
        toast({ description: t("github:failedToCheckForIssues"), variant: "error" });
      }
    },
    [trigger, t, toast, watches],
  );

  const handleReset = useCallback(
    async (id: string, workspaceId: string) => {
      try {
        const { tasksDeleted } = await reset(id, workspaceId);
        toast({
          description:
            tasksDeleted > 0
              ? t("github:resetCompleteDeletedTasks", { count: tasksDeleted })
              : t("github:resetCompleteNextPollWillRe"),
          variant: "success",
        });
      } catch {
        toast({ description: t("github:failedToResetIssueWatch"), variant: "error" });
        throw new Error("reset failed");
      }
    },
    [reset, t, toast],
  );

  return {
    watches,
    create,
    update,
    previewReset,
    handleDelete,
    handleTrigger,
    handleReset,
  };
}

/** Top-of-page GitHub settings: title + enable toggle, callback notice, and workspace credentials. */
export function GitHubConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <>
      <SettingsSection
        discoveryTargetId={INTEGRATION_SETTINGS_TARGETS.github}
        icon={<IconBrandGithub className="h-5 w-5" />}
        title={t("github:githubIntegration")}
        titleTestId="github-integration-heading"
        description={t("github:chooseTheAutomationAndPersonalIdentities")}
        action={<GitHubEnabledControl />}
      >
        <GitHubCallbackNotice workspaceId={workspaceId} />
        <SettingsSection
          title={t("github:workspaceGithubAccess")}
          description={t("github:credentialUsedForRepositorySyncWatches")}
        >
          <Card data-testid="github-workspace-access-card">
            <CardContent className="pt-6">
              <GitHubAutomationSettings workspaceId={workspaceId} />
            </CardContent>
          </Card>
        </SettingsSection>
      </SettingsSection>
      <div className="pr-16 sm:pr-0">
        <GitHubPersonalSettings workspaceId={workspaceId} />
      </div>
    </>
  );
}

function PerWorkspaceSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-8">
      <GitHubConnectionSection workspaceId={workspaceId} />
      <ReviewWatchSection workspaceId={workspaceId} />
      <IssueWatchSection workspaceId={workspaceId} />
      <GitHubRepoScopeSection workspaceId={workspaceId} />
      <ActionPresetsSection workspaceId={workspaceId} />
      <SettingsSection
        title={t("github:prAnalytics")}
        description={t("github:pullRequestActivityForThisWorkspace")}
      >
        <PRStatsPanel workspaceId={workspaceId} />
      </SettingsSection>
      <DefaultQueriesSection workspaceId={workspaceId} />
    </div>
  );
}

type GitHubIntegrationPageProps = {
  workspaceId?: string;
};

/** GitHub's own settings page: resolves the active workspace, then renders its connection, review/issue watch, repo-scope, and automation sections. */
export function GitHubIntegrationPage({ workspaceId }: GitHubIntegrationPageProps = {}) {
  return (
    <TooltipProvider>
      <div className="space-y-8 lg:pr-16">
        <WorkspaceScopedSection workspaceId={workspaceId}>
          {(ws) => <PerWorkspaceSection key={ws} workspaceId={ws} />}
        </WorkspaceScopedSection>
      </div>
    </TooltipProvider>
  );
}

function ReviewWatchSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { watches, create, update, previewReset, handleDelete, handleTrigger, handleReset } =
    useWatchActions(workspaceId);
  const { toast } = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingWatch] = useState<ReviewWatch | null>(null);
  const saveEnabled = useCallback(
    async (watch: ReviewWatch, enabled: boolean) => {
      await update(watch.id, watch.workspace_id, { enabled });
    },
    [update],
  );
  const enabledDrafts = useWatcherEnabledDrafts({
    id: `github-review-watch-enabled:${workspaceId}`,
    items: watches,
    saveEnabled,
  });
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
      const w = enabledDrafts.items.find((item) => item.id === id);
      if (w) setReviewResetting(w);
    },
    [enabledDrafts.items, setReviewResetting],
  );

  return (
    <>
      <SettingsSection
        title={t("github:reviewWatches")}
        description={t("github:automaticallyCreateTasksForPrsThat")}
        action={
          <div className="flex items-center gap-2">
            <CleanupNowButton
              label={t("github:cleanUpMerged")}
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
              {t("github:addWatch")}
            </Button>
          </div>
        }
      >
        <Card>
          <CardContent className="p-0">
            <ReviewWatchTable
              watches={enabledDrafts.items}
              onEdit={handleEdit}
              onDelete={handleDelete}
              onTrigger={handleTrigger}
              onReset={onResetClick}
              onToggleEnabled={enabledDrafts.toggleEnabled}
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
          toast({ description: t("github:reviewWatchCreated"), variant: "success" });
        }}
        onUpdate={async (id, req) => {
          const watch = watches.find((item) => item.id === id);
          // Unreachable-invariant guard: the dialog only ever calls onUpdate for
          // a row it was opened from. A developer diagnostic, not user copy.
          // eslint-disable-next-line i18next/no-literal-string -- invariant message
          if (!watch) throw new Error("review watch not found");
          await update(id, watch.workspace_id, req);
          toast({ description: t("github:reviewWatchUpdated"), variant: "success" });
        }}
      />
      {resetCtrl.resetting && (
        <ResetWatchDialog
          open
          onOpenChange={resetCtrl.onOpenChange}
          integrationLabel={t("github:reviewWatch")}
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </>
  );
}

function IssueWatchSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const issueActions = useIssueWatchActions(workspaceId);
  const { toast } = useToast();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingWatch, setEditingIssueWatch] = useState<IssueWatch | null>(null);
  const saveEnabled = useCallback(
    async (watch: IssueWatch, enabled: boolean) => {
      await issueActions.update(watch.id, watch.workspace_id, { enabled });
    },
    [issueActions.update],
  );
  const enabledDrafts = useWatcherEnabledDrafts({
    id: `github-issue-watch-enabled:${workspaceId}`,
    items: issueActions.watches,
    saveEnabled,
  });
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
      const w = enabledDrafts.items.find((item) => item.id === id);
      if (w) setIssueResetting(w);
    },
    [enabledDrafts.items, setIssueResetting],
  );

  return (
    <>
      <SettingsSection
        title={t("github:issueWatches")}
        description={t("github:automaticallyCreateTasksForGithubIssues")}
        action={
          <div className="flex items-center gap-2">
            <CleanupNowButton
              label={t("github:cleanUpClosed")}
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
              {t("github:addWatch")}
            </Button>
          </div>
        }
      >
        <Card>
          <CardContent className="p-0">
            <IssueWatchTable
              watches={enabledDrafts.items}
              onEdit={handleEdit}
              onDelete={issueActions.handleDelete}
              onTrigger={issueActions.handleTrigger}
              onReset={onResetClick}
              onToggleEnabled={enabledDrafts.toggleEnabled}
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
          toast({ description: t("github:issueWatchCreated"), variant: "success" });
        }}
        onUpdate={async (id, req) => {
          const watch = issueActions.watches.find((item) => item.id === id);
          // Unreachable-invariant guard, as above. Developer diagnostic.
          // eslint-disable-next-line i18next/no-literal-string -- invariant message
          if (!watch) throw new Error("issue watch not found");
          await issueActions.update(id, watch.workspace_id, req);
          toast({ description: t("github:issueWatchUpdated"), variant: "success" });
        }}
      />
      {resetCtrl.resetting && (
        <ResetWatchDialog
          open
          onOpenChange={resetCtrl.onOpenChange}
          integrationLabel={t("github:issueWatch")}
          previewLoader={resetCtrl.previewLoader}
          onConfirm={resetCtrl.confirmReset}
        />
      )}
    </>
  );
}
