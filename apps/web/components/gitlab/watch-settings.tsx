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
  return (
    <Button
      size="sm"
      onClick={onClick}
      className="min-h-11 w-full cursor-pointer sm:min-h-8 sm:w-auto"
    >
      <IconPlus className="mr-1 h-4 w-4" />
      New watch
    </Button>
  );
}

function useReviewActions(
  watches: ReviewWatches,
  workspaceId: string,
  setError: (message: string) => void,
) {
  const { toast } = useToast();
  const run = useCallback(
    async (watch: ReviewWatch) => {
      setError("");
      try {
        const result = await watches.trigger(watch.id, watch.workspace_id);
        toast({
          description: result.count
            ? `Found ${result.count} matching merge request(s)`
            : "No new merge requests matched",
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, "Review watch check failed"));
      }
    },
    [setError, toast, watches],
  );
  const remove = useCallback(
    async (watch: ReviewWatch) => {
      setError("");
      try {
        await watches.remove(watch.id, watch.workspace_id);
        toast({ description: "Review watch deleted", variant: "success" });
      } catch (error) {
        setError(errorMessage(error, "Review watch deletion failed"));
        throw error;
      }
    },
    [setError, toast, watches],
  );
  const reset = useWatchResetController<ReviewWatch>({
    preview: (watch) => watches.previewReset(watch.id, watch.workspace_id),
    reset: async (watch) => {
      setError("");
      try {
        const result = await watches.reset(watch.id, watch.workspace_id);
        toast({
          description: `Review watch reset; ${result.tasksDeleted} task(s) deleted`,
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, "Review watch reset failed"));
        throw error;
      }
    },
  });
  const create = useCallback(
    async (request: Parameters<ReviewWatches["create"]>[0]) => {
      setError("");
      try {
        await watches.create(request);
        toast({ description: "Review watch created", variant: "success" });
      } catch (error) {
        setError(errorMessage(error, "Review watch creation failed"));
        throw error;
      }
    },
    [setError, toast, watches],
  );
  const update = useCallback(
    async (id: string, request: Parameters<ReviewWatches["update"]>[1]) => {
      setError("");
      try {
        await watches.update(id, request, workspaceId);
        toast({ description: "Review watch updated", variant: "success" });
      } catch (error) {
        setError(errorMessage(error, "Review watch update failed"));
        throw error;
      }
    },
    [setError, toast, watches, workspaceId],
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
          integrationLabel="GitLab review watch"
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
          watchLabel="GitLab review watch"
          onConfirm={() => props.actions.remove(props.deleting!)}
        />
      )}
    </>
  );
}

function ReviewWatchSettings({ workspaceId }: { workspaceId: string }) {
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
      title="Merge request review watches"
      description="Poll GitLab for merge requests awaiting review and create one task per new match."
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
  const { toast } = useToast();
  const run = useCallback(
    async (watch: IssueWatch) => {
      setError("");
      try {
        const result = await watches.trigger(watch.id, watch.workspace_id);
        toast({
          description: result.count
            ? `Found ${result.count} matching issue(s)`
            : "No new issues matched",
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, "Issue watch check failed"));
      }
    },
    [setError, toast, watches],
  );
  const remove = useCallback(
    async (watch: IssueWatch) => {
      setError("");
      try {
        await watches.remove(watch.id, watch.workspace_id);
        toast({ description: "Issue watch deleted", variant: "success" });
      } catch (error) {
        setError(errorMessage(error, "Issue watch deletion failed"));
        throw error;
      }
    },
    [setError, toast, watches],
  );
  const reset = useWatchResetController<IssueWatch>({
    preview: (watch) => watches.previewReset(watch.id, watch.workspace_id),
    reset: async (watch) => {
      setError("");
      try {
        const result = await watches.reset(watch.id, watch.workspace_id);
        toast({
          description: `Issue watch reset; ${result.tasksDeleted} task(s) deleted`,
          variant: "success",
        });
      } catch (error) {
        setError(errorMessage(error, "Issue watch reset failed"));
        throw error;
      }
    },
  });
  const create = useCallback(
    async (request: Parameters<IssueWatches["create"]>[0]) => {
      setError("");
      try {
        await watches.create(request);
        toast({ description: "Issue watch created", variant: "success" });
      } catch (error) {
        setError(errorMessage(error, "Issue watch creation failed"));
        throw error;
      }
    },
    [setError, toast, watches],
  );
  const update = useCallback(
    async (id: string, request: Parameters<IssueWatches["update"]>[1]) => {
      setError("");
      try {
        await watches.update(id, request, workspaceId);
        toast({ description: "Issue watch updated", variant: "success" });
      } catch (error) {
        setError(errorMessage(error, "Issue watch update failed"));
        throw error;
      }
    },
    [setError, toast, watches, workspaceId],
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
          integrationLabel="GitLab issue watch"
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
          watchLabel="GitLab issue watch"
          onConfirm={() => props.actions.remove(props.deleting!)}
        />
      )}
    </>
  );
}

function IssueWatchSettings({ workspaceId }: { workspaceId: string }) {
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
      title="Issue watches"
      description="Poll GitLab issues and create one task per new match."
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
