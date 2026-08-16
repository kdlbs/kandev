"use client";

import { useEffect, useMemo, useState } from "react";
import {
  IconBrandGitlab,
  IconCheck,
  IconExternalLink,
  IconGitMerge,
  IconLink,
  IconLoader2,
  IconRefresh,
  IconUnlink,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Input } from "@kandev/ui/input";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Separator } from "@kandev/ui/separator";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useAppStore } from "@/components/state-provider";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useAddMRFeedbackAsContext, useMRFeedback } from "@/hooks/domains/gitlab/use-mr-feedback";
import { useMRActions } from "@/hooks/domains/gitlab/use-mr-actions";
import { useGitLabStatus } from "@/hooks/domains/gitlab/use-gitlab-status";
import {
  approveMR,
  createMRDiscussionNote,
  deleteTaskMR,
  mergeMR,
  resolveMRDiscussion,
  setMRAssignees,
  setMRLabels,
  setMRReviewers,
  unapproveMR,
} from "@/lib/api/domains/gitlab-api";
import type { GitLabMRIdentity } from "@/lib/api/domains/gitlab-api";
import { normalizeGitLabOrigin } from "@/lib/gitlab-identity";
import { openExternalLink } from "@/lib/desktop/external-links";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { TaskMR } from "@/lib/types/gitlab";
import { MROverviewSection } from "./mr-overview-section";
import { MRFilesSection } from "./mr-files-section";
import { MRCommitsSection } from "./mr-commits-section";
import { MRDiscussionsSection } from "./mr-discussions-section";
import { MRReviewerControl } from "./mr-reviewer-control";
import { SubscriptionToggle } from "./subscription-toggle";
import { useTranslation } from "react-i18next";

type MRKeyInput = Pick<TaskMR, "host" | "project_path" | "mr_iid">;
const EMPTY_LABELS: string[] = [];
type RunMRAction = ReturnType<typeof useMRActions>["run"];

export function mrTaskKey(mr: MRKeyInput): string {
  return `${normalizeGitLabOrigin(mr.host)}|${mr.project_path}|${mr.mr_iid}`;
}

export function isOpenMRState(state: string): boolean {
  return state === "open" || state === "opened";
}

export function selectPanelMR(mrs: TaskMR[], mrKey?: string): TaskMR | null {
  if (!mrKey) return mrs[0] ?? null;
  return mrs.find((mr) => mrTaskKey(mr) === mrKey) ?? null;
}

export function selectExplicitPanelMR(mrs: TaskMR[], mrKey: string | null): TaskMR | null {
  if (!mrKey) return null;
  return mrs.find((mr) => mrTaskKey(mr) === mrKey) ?? null;
}

export function isTaskMRHostAllowed(taskMRHost: string, configuredHost: string | null): boolean {
  if (!configuredHost) return false;
  const taskOrigin = normalizeGitLabOrigin(taskMRHost);
  const configuredOrigin = normalizeGitLabOrigin(configuredHost);
  return !!taskOrigin && taskOrigin === configuredOrigin;
}

function usePanelMR(mrKey?: string) {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const mrs = useTaskMRs(taskId);
  return selectPanelMR(mrs, mrKey);
}

function LabelsControl({
  value,
  busy,
  onSave,
}: {
  value: string[];
  busy: boolean;
  onSave: (labels: string[]) => void;
}) {
  const { t } = useTranslation();
  const [labels, setLabels] = useState(value.join(", "));
  useEffect(() => setLabels(value.join(", ")), [value]);
  return (
    <section className="space-y-2">
      <h4 className="text-xs font-semibold">{t("gitlab:labels")}</h4>
      <div className="flex gap-2">
        <Input
          value={labels}
          onChange={(event) => setLabels(event.target.value)}
          placeholder={t("gitlab:bugBackend")}
          aria-label={t("gitlab:mergeRequestLabels")}
        />
        <Button
          size="sm"
          variant="outline"
          className="h-11 shrink-0 cursor-pointer sm:h-9"
          disabled={busy}
          onClick={() =>
            onSave(
              labels
                .split(",")
                .map((label) => label.trim())
                .filter(Boolean),
            )
          }
        >
          {t("gitlab:apply")}
        </Button>
      </div>
    </section>
  );
}

function PanelHeader({
  mr,
  loading,
  onRefresh,
  onUnlink,
}: {
  mr: TaskMR;
  loading: boolean;
  onRefresh: () => void;
  onUnlink: () => void;
}) {
  const { t } = useTranslation();
  return (
    <header className="border-b px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        <IconBrandGitlab className="h-4 w-4 shrink-0 text-orange-500" />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-sm font-semibold">
            {mr.mr_title || t("gitlab:mergeRequest", { mriid: mr.mr_iid })}
          </h2>
          <p className="truncate text-[10px] text-muted-foreground">
            {mr.project_path}!{mr.mr_iid}
          </p>
        </div>
        <Button
          size="icon-sm"
          variant="ghost"
          className="h-10 w-10 cursor-pointer sm:h-8 sm:w-8"
          aria-label={t("gitlab:refreshMergeRequest")}
          onClick={onRefresh}
        >
          <IconRefresh className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
        </Button>
        <Button
          size="icon-sm"
          variant="ghost"
          className="h-10 w-10 cursor-pointer sm:h-8 sm:w-8"
          aria-label={t("gitlab:openMergeRequestInGitlab")}
          onClick={() => void openExternalLink(mr.mr_url).catch(() => undefined)}
        >
          <IconExternalLink className="h-4 w-4" />
        </Button>
        <Button
          size="icon-sm"
          variant="ghost"
          className="h-10 w-10 cursor-pointer text-destructive sm:h-8 sm:w-8"
          aria-label={t("gitlab:unlinkMergeRequest")}
          onClick={onUnlink}
        >
          <IconUnlink className="h-4 w-4" />
        </Button>
      </div>
    </header>
  );
}

function MRActionButtons({
  identity,
  state,
  hasConflicts,
  busy,
  run,
  onMerge,
}: {
  identity: GitLabMRIdentity & { host: string };
  state: string;
  hasConflicts: boolean;
  busy: boolean;
  run: RunMRAction;
  onMerge: () => void;
}) {
  const { t } = useTranslation();
  const disabled = busy || !isOpenMRState(state);
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        className="h-11 cursor-pointer gap-1.5 bg-green-600 text-white hover:bg-green-700 sm:h-8"
        disabled={disabled}
        onClick={() => void run("approve", () => approveMR(identity))}
      >
        <IconCheck className="h-4 w-4" /> {t("gitlab:approve")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="h-11 cursor-pointer sm:h-8"
        disabled={disabled}
        onClick={() => void run("unapprove", () => unapproveMR(identity))}
      >
        {t("gitlab:unapprove")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="h-11 cursor-pointer gap-1.5 sm:h-8"
        disabled={disabled || hasConflicts}
        onClick={onMerge}
      >
        <IconGitMerge className="h-4 w-4" /> {t("gitlab:merge")}
      </Button>
      <SubscriptionToggle {...identity} />
    </div>
  );
}

function MRPeopleControls({
  identity,
  reviewers,
  assignees,
  busy,
  run,
}: {
  identity: GitLabMRIdentity & { host: string };
  reviewers: Parameters<typeof MRReviewerControl>[0]["current"];
  assignees: Parameters<typeof MRReviewerControl>[0]["current"];
  busy: boolean;
  run: RunMRAction;
}) {
  const shared = {
    workspaceId: identity.workspaceId,
    host: identity.host,
    project: identity.project,
  };
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <MRReviewerControl
        {...shared}
        kind="reviewers"
        current={reviewers}
        busy={busy}
        onSave={(reviewerIds) =>
          run("reviewers", () => setMRReviewers({ ...identity, reviewerIds }))
        }
      />
      <MRReviewerControl
        {...shared}
        kind="assignees"
        current={assignees}
        busy={busy}
        onSave={(assigneeIds) =>
          run("assignees", () => setMRAssignees({ ...identity, assigneeIds }))
        }
      />
    </div>
  );
}

function MergeConfirmation({
  open,
  onOpenChange,
  taskMR,
  baseBranch,
  identity,
  run,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskMR: TaskMR;
  baseBranch: string;
  identity: GitLabMRIdentity;
  run: RunMRAction;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="w-[calc(100vw-2rem)] sm:max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("gitlab:mergeThisMergeRequest")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("gitlab:gitlabWillMergeDescription", {
              mr: `${taskMR.project_path}!${taskMR.mr_iid}`,
              baseBranch,
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            onClick={() => void run("merge", () => mergeMR({ ...identity, squash: false }))}
          >
            {t("gitlab:merge")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function FeedbackPlaceholder({ error, onRetry }: { error: string | null; onRetry: () => void }) {
  const { t } = useTranslation();
  if (error) {
    return (
      <div className="p-4 text-sm text-destructive">
        {error}
        <Button variant="outline" size="sm" className="ml-2 cursor-pointer" onClick={onRetry}>
          {t("gitlab:retry")}
        </Button>
      </div>
    );
  }
  return (
    <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
      <IconLoader2 className="h-4 w-4 animate-spin" /> {t("gitlab:loadingMergeRequest")}
    </div>
  );
}

async function unlinkTaskMR(
  panelId: string,
  taskMR: TaskMR,
  workspaceId: string,
  run: RunMRAction,
  removeTaskMR: (workspaceId: string, associationId: string) => void,
) {
  if (!(await run("unlink", () => deleteTaskMR(taskMR.id, workspaceId)))) {
    return;
  }
  removeTaskMR(workspaceId, taskMR.id);
  const api = useDockviewStore.getState().api;
  const panel = api?.getPanel(panelId);
  if (api && panel) api.removePanel(panel);
}

/**
 * The two discussion callbacks, lifted out of MRDetailContent to keep it under
 * the 100-line function cap. The action names and toast strings stay English:
 * they are `useMRActions` bookkeeping, not copy a user reads.
 */
function discussionHandlers(identity: GitLabMRIdentity & { host: string }, run: RunMRAction) {
  return {
    reply: (discussionId: string, body: string) =>
      run("reply", () => createMRDiscussionNote({ ...identity, discussionId, body })),
    resolve: (discussionId: string) =>
      run("resolve", () => resolveMRDiscussion({ ...identity, discussionId })),
  };
}

function MRDetailContent({
  panelId,
  taskMR,
  sessionId,
  workspaceId,
}: {
  panelId: string;
  taskMR: TaskMR;
  sessionId: string;
  workspaceId: string;
}) {
  const { t } = useTranslation();
  const [mergeOpen, setMergeOpen] = useState(false);
  const removeTaskMR = useAppStore((state) => state.removeTaskMR);
  const { feedback, files, commits, loading, error, refresh } = useMRFeedback(
    workspaceId,
    taskMR.project_path,
    taskMR.mr_iid,
    taskMR.host,
  );
  const { pendingAction, run } = useMRActions(refresh);
  const addContext = useAddMRFeedbackAsContext(sessionId, taskMR.project_path, taskMR.mr_iid);
  const identity = useMemo<GitLabMRIdentity & { host: string }>(
    () => ({ workspaceId, project: taskMR.project_path, iid: taskMR.mr_iid, host: taskMR.host }),
    [workspaceId, taskMR.project_path, taskMR.mr_iid, taskMR.host],
  );
  const busy = pendingAction !== null;
  if (!feedback) return <FeedbackPlaceholder error={error} onRetry={refresh} />;
  const mr = feedback.mr;
  const { reply, resolve } = discussionHandlers(identity, run);
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="mr-detail-panel">
      <PanelHeader
        mr={taskMR}
        loading={loading}
        onRefresh={refresh}
        onUnlink={() => void unlinkTaskMR(panelId, taskMR, workspaceId, run, removeTaskMR)}
      />
      <ScrollArea className="min-h-0 flex-1">
        <div className="mx-auto w-full max-w-5xl space-y-4 p-3 sm:p-4">
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t("gitlab:refreshFailedShowingCached", { error })}
              </AlertDescription>
            </Alert>
          ) : null}
          <MROverviewSection taskMR={taskMR} feedback={feedback} />
          <MRActionButtons
            identity={identity}
            state={mr.state}
            hasConflicts={mr.has_conflicts}
            busy={busy}
            run={run}
            onMerge={() => setMergeOpen(true)}
          />
          <Separator />
          <MRPeopleControls
            identity={identity}
            reviewers={mr.reviewers ?? []}
            assignees={mr.assignees ?? []}
            busy={busy}
            run={run}
          />
          <LabelsControl
            value={mr.labels ?? EMPTY_LABELS}
            busy={busy}
            onSave={(labels) => void run("labels", () => setMRLabels({ ...identity, labels }))}
          />
          <Separator />
          <MRFilesSection files={files} />
          <MRCommitsSection commits={commits} />
          <MRDiscussionsSection
            discussions={feedback.discussions ?? []}
            mrUrl={taskMR.mr_url}
            busy={busy}
            onReply={reply}
            onResolve={resolve}
            onAddContext={addContext}
          />
          {error && (
            <p className="text-xs text-destructive">{t("gitlab:refreshFailed", { error })}</p>
          )}
        </div>
      </ScrollArea>
      <MergeConfirmation
        open={mergeOpen}
        onOpenChange={setMergeOpen}
        taskMR={taskMR}
        baseBranch={mr.base_branch}
        identity={identity}
        run={run}
      />
    </div>
  );
}

export function MRDetailPanelComponent({
  panelId,
  params,
}: {
  panelId: string;
  params?: { mrKey?: string };
}) {
  const { t } = useTranslation();
  const mr = usePanelMR(params?.mrKey);
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { status, loading } = useGitLabStatus();
  useEffect(
    () => setPanelTitle(panelId, mr ? `MR !${mr.mr_iid}` : t("task:panelMergeRequest")),
    [mr, panelId, t],
  );
  if (!mr || !workspaceId || !sessionId)
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        <IconLink className="mr-2 h-4 w-4" />
        {t("gitlab:noMergeRequestLinked")}
      </div>
    );
  if (loading)
    return (
      <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        {t("gitlab:verifyingGitlabConnection")}
      </div>
    );
  if (!isTaskMRHostAllowed(mr.host, status?.host ?? null)) {
    return (
      <div className="flex h-full items-center justify-center p-4 text-center text-sm text-destructive">
        {t("gitlab:thisMergeRequestBelongsToA")}
      </div>
    );
  }
  return (
    <MRDetailContent
      panelId={panelId}
      taskMR={mr}
      sessionId={sessionId}
      workspaceId={workspaceId}
    />
  );
}
