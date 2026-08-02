"use client";

import { useCallback, useState } from "react";
import { IconAlertTriangle, IconGitMerge, IconPlus, IconTicket } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { WatcherSettingsCard } from "@/components/integrations/watcher-settings-card";
import { useWatcherEnabledDrafts } from "@/components/integrations/use-watcher-enabled-drafts";
import { SettingsSection } from "@/components/settings/settings-section";
import { useToast } from "@/components/toast-provider";
import { ResetWatchDialog, useWatchResetController } from "@/components/watches/reset-watch-dialog";
import { useGitLabIssueWatches } from "@/hooks/domains/gitlab/use-gitlab-issue-watches";
import { useGitLabReviewWatches } from "@/hooks/domains/gitlab/use-gitlab-review-watches";
import type { IssueWatch, ReviewWatch } from "@/lib/types/gitlab";
import { IssueWatchDialog } from "./issue-watch-dialog";
import { IssueWatchTable } from "./issue-watch-table";
import { ReviewWatchDialog } from "./review-watch-dialog";
import { ReviewWatchTable } from "./review-watch-table";
import { DeleteWatchDialog } from "./delete-watch-dialog";
import { useTranslation } from "react-i18next";

type ReviewWatches = ReturnType<typeof useGitLabReviewWatches>;
type IssueWatches = ReturnType<typeof useGitLabIssueWatches>;

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

function ActionError({ message }: { message: string }) {
  if (!message) return null;
  return (
    <Alert variant="destructive">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}

function NewWatchButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      size="sm"
      onClick={onClick}
      className="min-h-11 w-full cursor-pointer sm:min-h-8 sm:w-auto"
    >
      <IconPlus className="mr-1 h-4 w-4" />
      {t("gitlab:newWatch")}
    </Button>
  );
}

function useReviewActions(
  watches: ReviewWatches,
  workspaceId: string,
  setError: (message: string) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const run = useCallback(
    async (watch: ReviewWatch) => {
      setError("");
      try {
        const result = await watches.trigger(watch.id, watch.workspace_id);
        toast({
          description: result.count
            ? t("gitlab:foundMatchingMergeRequests", { count: result.count })
            : t("gitlab:noNewMergeRequestsMatched"),
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:reviewWatchCheckFailed")));
      }
    },
    [setError, t, toast, watches],
  );
  const remove = useCallback(
    async (watch: ReviewWatch) => {
      setError("");
      try {
        await watches.remove(watch.id, watch.workspace_id);
        toast({ description: t("gitlab:reviewWatchDeleted"), variant: "success" });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:reviewWatchDeletionFailed")));
        throw error;
      }
    },
    [setError, t, toast, watches],
  );
  const reset = useWatchResetController<ReviewWatch>({
    preview: (watch) => watches.previewReset(watch.id, watch.workspace_id),
    reset: async (watch) => {
      setError("");
      try {
        const result = await watches.reset(watch.id, watch.workspace_id);
        toast({
          description: t("gitlab:reviewWatchReset", { count: result.tasksDeleted }),
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:reviewWatchResetFailed")));
        throw error;
      }
    },
  });
  const create = useCallback(
    async (request: Parameters<ReviewWatches["create"]>[0]) => {
      setError("");
      try {
        await watches.create(request);
        toast({ description: t("gitlab:reviewWatchCreated"), variant: "success" });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:reviewWatchCreationFailed")));
        throw error;
      }
    },
    [setError, t, toast, watches],
  );
  const update = useCallback(
    async (id: string, request: Parameters<ReviewWatches["update"]>[1]) => {
      setError("");
      try {
        await watches.update(id, request, workspaceId);
        toast({ description: t("gitlab:reviewWatchUpdated"), variant: "success" });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:reviewWatchUpdateFailed")));
        throw error;
      }
    },
    [setError, t, toast, watches, workspaceId],
  );
  return { run, remove, reset, create, update };
}

function ReviewDialogs(props: {
  workspaceId: string;
  open: boolean;
  setOpen: (open: boolean) => void;
  editing: ReviewWatch | null;
  deleting: ReviewWatch | null;
  setDeleting: (watch: ReviewWatch | null) => void;
  actions: ReturnType<typeof useReviewActions>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <ReviewWatchDialog
        open={props.open}
        onOpenChange={props.setOpen}
        watch={props.editing}
        workspaceId={props.workspaceId}
        onCreate={props.actions.create}
        onUpdate={props.actions.update}
      />
      {props.actions.reset.resetting && (
        <ResetWatchDialog
          open
          requirePreviewSuccess
          onOpenChange={props.actions.reset.onOpenChange}
          integrationLabel={t("gitlab:gitlabReviewWatch")}
          previewLoader={props.actions.reset.previewLoader}
          onConfirm={props.actions.reset.confirmReset}
        />
      )}
      {props.deleting && (
        <DeleteWatchDialog
          open
          onOpenChange={(open) => {
            if (!open) props.setDeleting(null);
          }}
          watchLabel={t("gitlab:gitlabReviewWatch")}
          onConfirm={() => props.actions.remove(props.deleting!)}
        />
      )}
    </>
  );
}

function ReviewWatchSettings({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const watches = useGitLabReviewWatches(workspaceId);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ReviewWatch | null>(null);
  const [deleting, setDeleting] = useState<ReviewWatch | null>(null);
  const [actionError, setActionError] = useState("");
  const actions = useReviewActions(watches, workspaceId, setActionError);
  const enabled = useWatcherEnabledDrafts({
    id: `gitlab-review-watch-enabled-${workspaceId}`,
    items: watches.items,
    saveEnabled: (watch, value) =>
      watches.update(watch.id, { enabled: value }, watch.workspace_id).then(() => undefined),
  });
  const find = (id: string) => watches.items.find((watch) => watch.id === id);
  return (
    <SettingsSection
      icon={<IconGitMerge className="h-5 w-5" />}
      title={t("gitlab:mergeRequestReviewWatches")}
      description={t("gitlab:pollGitlabForMergeRequestsAwaiting")}
      action={
        <NewWatchButton
          onClick={() => {
            setEditing(null);
            setDialogOpen(true);
          }}
        />
      }
    >
      <ActionError message={actionError} />
      <WatcherSettingsCard
        isDirty={enabled.dirtyIds.size > 0}
        isLoading={watches.loading}
        isEmpty={watches.items.length === 0}
        testId="gitlab-review-watches-card"
      >
        <ReviewWatchTable
          watches={enabled.items}
          dirtyIds={enabled.dirtyIds}
          authoritativeEnabledById={
            new Map(watches.items.map((watch) => [watch.id, watch.enabled]))
          }
          onEdit={(watch) => {
            setEditing(watch);
            setDialogOpen(true);
          }}
          onDelete={(id) => {
            const watch = find(id);
            if (watch) setDeleting(watch);
          }}
          onTrigger={(id) => {
            const watch = find(id);
            if (watch) void actions.run(watch);
          }}
          onReset={(id) => {
            const watch = find(id);
            if (watch) actions.reset.setResetting(watch);
          }}
          onToggleEnabled={enabled.toggleEnabled}
        />
      </WatcherSettingsCard>
      <ReviewDialogs
        workspaceId={workspaceId}
        open={dialogOpen}
        setOpen={setDialogOpen}
        editing={editing}
        deleting={deleting}
        setDeleting={setDeleting}
        actions={actions}
      />
    </SettingsSection>
  );
}

function useIssueActions(
  watches: IssueWatches,
  workspaceId: string,
  setError: (message: string) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const run = useCallback(
    async (watch: IssueWatch) => {
      setError("");
      try {
        const result = await watches.trigger(watch.id, watch.workspace_id);
        toast({
          description: result.count
            ? t("gitlab:foundMatchingIssues", { count: result.count })
            : t("gitlab:noNewIssuesMatched"),
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:issueWatchCheckFailed")));
      }
    },
    [setError, t, toast, watches],
  );
  const remove = useCallback(
    async (watch: IssueWatch) => {
      setError("");
      try {
        await watches.remove(watch.id, watch.workspace_id);
        toast({ description: t("gitlab:issueWatchDeleted"), variant: "success" });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:issueWatchDeletionFailed")));
        throw error;
      }
    },
    [setError, t, toast, watches],
  );
  const reset = useWatchResetController<IssueWatch>({
    preview: (watch) => watches.previewReset(watch.id, watch.workspace_id),
    reset: async (watch) => {
      setError("");
      try {
        const result = await watches.reset(watch.id, watch.workspace_id);
        toast({
          description: t("gitlab:issueWatchReset", { count: result.tasksDeleted }),
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:issueWatchResetFailed")));
        throw error;
      }
    },
  });
  const create = useCallback(
    async (request: Parameters<IssueWatches["create"]>[0]) => {
      setError("");
      try {
        await watches.create(request);
        toast({ description: t("gitlab:issueWatchCreated"), variant: "success" });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:issueWatchCreationFailed")));
        throw error;
      }
    },
    [setError, t, toast, watches],
  );
  const update = useCallback(
    async (id: string, request: Parameters<IssueWatches["update"]>[1]) => {
      setError("");
      try {
        await watches.update(id, request, workspaceId);
        toast({ description: t("gitlab:issueWatchUpdated"), variant: "success" });
      } catch (error) {
        setError(errorMessage(error, t("gitlab:issueWatchUpdateFailed")));
        throw error;
      }
    },
    [setError, t, toast, watches, workspaceId],
  );
  return { run, remove, reset, create, update };
}

function IssueDialogs(props: {
  workspaceId: string;
  open: boolean;
  setOpen: (open: boolean) => void;
  editing: IssueWatch | null;
  deleting: IssueWatch | null;
  setDeleting: (watch: IssueWatch | null) => void;
  actions: ReturnType<typeof useIssueActions>;
}) {
  const { t } = useTranslation();
  return (
    <>
      <IssueWatchDialog
        open={props.open}
        onOpenChange={props.setOpen}
        watch={props.editing}
        workspaceId={props.workspaceId}
        onCreate={props.actions.create}
        onUpdate={props.actions.update}
      />
      {props.actions.reset.resetting && (
        <ResetWatchDialog
          open
          requirePreviewSuccess
          onOpenChange={props.actions.reset.onOpenChange}
          integrationLabel={t("gitlab:gitlabIssueWatch")}
          previewLoader={props.actions.reset.previewLoader}
          onConfirm={props.actions.reset.confirmReset}
        />
      )}
      {props.deleting && (
        <DeleteWatchDialog
          open
          onOpenChange={(open) => {
            if (!open) props.setDeleting(null);
          }}
          watchLabel={t("gitlab:gitlabIssueWatch")}
          onConfirm={() => props.actions.remove(props.deleting!)}
        />
      )}
    </>
  );
}

function IssueWatchSettings({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const watches = useGitLabIssueWatches(workspaceId);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<IssueWatch | null>(null);
  const [deleting, setDeleting] = useState<IssueWatch | null>(null);
  const [actionError, setActionError] = useState("");
  const actions = useIssueActions(watches, workspaceId, setActionError);
  const enabled = useWatcherEnabledDrafts({
    id: `gitlab-issue-watch-enabled-${workspaceId}`,
    items: watches.items,
    saveEnabled: (watch, value) =>
      watches.update(watch.id, { enabled: value }, watch.workspace_id).then(() => undefined),
  });
  const find = (id: string) => watches.items.find((watch) => watch.id === id);
  return (
    <SettingsSection
      icon={<IconTicket className="h-5 w-5" />}
      title={t("gitlab:issueWatches")}
      description={t("gitlab:pollGitlabIssuesAndCreateOne")}
      action={
        <NewWatchButton
          onClick={() => {
            setEditing(null);
            setDialogOpen(true);
          }}
        />
      }
    >
      <ActionError message={actionError} />
      <WatcherSettingsCard
        isDirty={enabled.dirtyIds.size > 0}
        isLoading={watches.loading}
        isEmpty={watches.items.length === 0}
        testId="gitlab-issue-watches-card"
      >
        <IssueWatchTable
          watches={enabled.items}
          dirtyIds={enabled.dirtyIds}
          authoritativeEnabledById={
            new Map(watches.items.map((watch) => [watch.id, watch.enabled]))
          }
          onEdit={(watch) => {
            setEditing(watch);
            setDialogOpen(true);
          }}
          onDelete={(id) => {
            const watch = find(id);
            if (watch) setDeleting(watch);
          }}
          onTrigger={(id) => {
            const watch = find(id);
            if (watch) void actions.run(watch);
          }}
          onReset={(id) => {
            const watch = find(id);
            if (watch) actions.reset.setResetting(watch);
          }}
          onToggleEnabled={enabled.toggleEnabled}
        />
      </WatcherSettingsCard>
      <IssueDialogs
        workspaceId={workspaceId}
        open={dialogOpen}
        setOpen={setDialogOpen}
        editing={editing}
        deleting={deleting}
        setDeleting={setDeleting}
        actions={actions}
      />
    </SettingsSection>
  );
}

export function GitLabWatchSettings({ workspaceId }: { workspaceId: string }) {
  return (
    <>
      <ReviewWatchSettings workspaceId={workspaceId} />
      <IssueWatchSettings workspaceId={workspaceId} />
    </>
  );
}
