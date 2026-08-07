"use client";

import { memo, useEffect, useState } from "react";
import Link from "@/components/routing/app-link";
import {
  IconBrandGitlab,
  IconCheck,
  IconClock,
  IconExternalLink,
  IconGitMerge,
  IconPlus,
  IconUnlink,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  useGitLabAvailable,
  useTaskMRs,
  useWorkspaceMRs,
} from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { deleteTaskMR } from "@/lib/api/domains/gitlab-api";
import type { TaskMR } from "@/lib/types/gitlab";
import type { Repository } from "@/lib/types/http";
import { TaskMRLinkDialog } from "./task-mr-link-dialog";
import { MRAutomationControls } from "./mr-automation-controls";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { reviewItemId } from "@/components/task/review-selection";
import { mrTaskKey } from "./mr-detail-panel";
import { useTranslation } from "react-i18next";

/**
 * Icon + colour for an MR's combined state. Mirrors github's pr-task-icon
 * priority order so a merged MR reads the same as a merged PR: terminal
 * states first, then pipeline failures / changes-requested, then ready-to-
 * merge, then awaiting-something, then pipeline-running.
 */
function MRStatusIcon({ mr }: { mr: TaskMR }) {
  if (mr.state === "merged") return <IconCheck className="h-3 w-3 text-purple-500" />;
  if (mr.state === "closed") return <IconX className="h-3 w-3 text-muted-foreground" />;
  if (mr.pipeline_state === "failure") return <IconX className="h-3 w-3 text-red-500" />;
  if (mr.approval_state === "approved" && mr.pipeline_state === "success" && !mr.draft) {
    return <IconCheck className="h-3 w-3 text-emerald-400" />;
  }
  if (mr.approval_state === "pending") return <IconClock className="h-3 w-3 text-sky-400" />;
  if (mr.pipeline_state === "pending") return <IconClock className="h-3 w-3 text-yellow-500" />;
  return null;
}

function statusTextColor(mr: TaskMR): string {
  if (mr.state === "merged") return "text-purple-500";
  if (mr.state === "closed") return "text-muted-foreground";
  if (mr.pipeline_state === "failure") return "text-red-500";
  if (mr.approval_state === "approved") return "text-emerald-400";
  if (mr.approval_state === "pending") return "text-sky-400";
  return "text-muted-foreground";
}

export function mrTriggerClass(compact: boolean, mobile: boolean): string {
  if (mobile) return "h-11 w-11 cursor-pointer";
  return compact ? "h-9 w-9 cursor-pointer" : "cursor-pointer gap-1.5 px-2";
}

export function openMobileMRReview(
  setReview: (sessionId: string, mrKey: string) => void,
  sessionId: string,
  mr: TaskMR,
) {
  setReview(sessionId, reviewItemId({ providerId: "gitlab", reviewKey: mrTaskKey(mr) }));
}

export function openDesktopMRReview(
  addMRPanel: (mrKey: string) => void,
  mr: TaskMR,
  schedule: (callback: FrameRequestCallback) => number = requestAnimationFrame,
) {
  const mrKey = mrTaskKey(mr);
  addMRPanel(mrKey);
  schedule(() => {
    schedule(() => addMRPanel(mrKey));
  });
}

function MRTriggerContent({
  compact,
  single,
  count,
}: {
  compact: boolean;
  single: TaskMR | null;
  count: number;
}) {
  const { t } = useTranslation();
  if (compact) return <IconBrandGitlab className="h-4 w-4 text-orange-500" />;
  if (single) {
    return (
      <>
        <IconGitMerge className={`h-4 w-4 ${statusTextColor(single)}`} />
        <span className="text-xs font-medium">!{single.mr_iid}</span>
        <MRStatusIcon mr={single} />
      </>
    );
  }
  return (
    <>
      <IconBrandGitlab className="h-4 w-4 text-orange-500" />
      <span className="text-xs font-medium">
        {count} {t("gitlab:mrs")}
      </span>
    </>
  );
}

function MRMenuButton({
  taskId,
  mrs,
  canLink,
  compact,
  mobile,
  onLink,
  onUnlink,
}: {
  taskId: string;
  mrs: TaskMR[];
  canLink: boolean;
  compact: boolean;
  mobile: boolean;
  onLink: () => void;
  onUnlink: (associationId: string) => void;
}) {
  const { t } = useTranslation();
  const single = mrs.length === 1 ? mrs[0] : null;
  const addMRPanel = useDockviewStore((state) => state.addMRPanel);
  const dockviewReady = useDockviewStore((state) => state.api !== null);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setMobileSessionReview = useAppStore((state) => state.setMobileSessionReview);
  const [pendingDesktopMR, setPendingDesktopMR] = useState<TaskMR | null>(null);

  useEffect(() => {
    if (!dockviewReady || !pendingDesktopMR) return;
    openDesktopMRReview(addMRPanel, pendingDesktopMR);
    setPendingDesktopMR(null);
  }, [addMRPanel, dockviewReady, pendingDesktopMR]);

  const openReview = (mr: TaskMR) => {
    if (mobile) {
      if (activeSessionId) openMobileMRReview(setMobileSessionReview, activeSessionId, mr);
      return;
    }
    if (!dockviewReady) {
      setPendingDesktopMR(mr);
    } else {
      openDesktopMRReview(addMRPanel, mr);
    }
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          data-testid="mr-topbar-button"
          data-mr-iid={single?.mr_iid}
          data-mr-state={single?.state}
          size={compact ? "icon-sm" : "sm"}
          variant="outline"
          className={mrTriggerClass(compact, mobile)}
          aria-label={
            single
              ? t("gitlab:gitlabMergeRequest", { mriid: single.mr_iid })
              : t("gitlab:gitlabMergeRequests", { length: mrs.length })
          }
        >
          <MRTriggerContent compact={compact} single={single} count={mrs.length} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        {mrs.map((mr) => (
          <div key={mr.id}>
            <DropdownMenuItem className="cursor-pointer" onSelect={() => openReview(mr)}>
              <IconGitMerge className="h-4 w-4" />
              <span className="min-w-0 truncate">
                {t("gitlab:review")} {mr.project_path}!{mr.mr_iid}
              </span>
            </DropdownMenuItem>
            <DropdownMenuItem asChild className="cursor-pointer">
              <Link href={mr.mr_url} target="_blank" rel="noopener noreferrer">
                <IconExternalLink className="h-4 w-4" /> {t("gitlab:openInGitlab")}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer text-destructive focus:text-destructive"
              onSelect={() => onUnlink(mr.id)}
            >
              <IconUnlink className="h-4 w-4" />
              {t("gitlab:unlink")}
              {mr.mr_iid}
            </DropdownMenuItem>
          </div>
        ))}
        <DropdownMenuSeparator />
        <MRAutomationControls taskId={taskId} />
        {canLink ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem className="cursor-pointer" onSelect={onLink}>
              <IconPlus className="h-4 w-4" />
              {t("gitlab:linkAnotherMergeRequest")}
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

const EMPTY_REPOSITORIES: Repository[] = [];
const EMPTY_TASK_REPOSITORIES: Array<{ repository_id: string }> = [];

function MRTopbarControl({
  taskId,
  mrs,
  gitlabAvailable,
  compact,
  mobile,
  onLink,
  onUnlink,
}: {
  taskId: string;
  mrs: TaskMR[];
  gitlabAvailable: boolean;
  compact: boolean;
  mobile: boolean;
  onLink: () => void;
  onUnlink: (associationId: string) => void;
}) {
  if (mrs.length > 0) {
    return (
      <MRMenuButton
        taskId={taskId}
        mrs={mrs}
        canLink={gitlabAvailable}
        compact={compact}
        mobile={mobile}
        onLink={onLink}
        onUnlink={onUnlink}
      />
    );
  }
  return null;
}

export const MRTopbarButton = memo(function MRTopbarButton({
  compact = false,
  mobile = false,
}: {
  compact?: boolean;
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  const [linkOpen, setLinkOpen] = useState(false);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const repositories = useAppStore((state) =>
    workspaceId
      ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
  const task = useTaskById(activeTaskId);
  useWorkspaceMRs(workspaceId);
  const mrs = useTaskMRs(activeTaskId);
  const gitlabAvailable = useGitLabAvailable();
  const removeTaskMR = useAppStore((state) => state.removeTaskMR);
  const { toast } = useToast();

  if (!activeTaskId || !workspaceId) return null;

  const unlink = async (associationId: string) => {
    try {
      await deleteTaskMR(associationId, workspaceId);
      removeTaskMR(workspaceId, associationId);
    } catch (error) {
      toast({
        title: t("gitlab:failedToUnlinkMergeRequest"),
        description:
          error instanceof Error ? error.message : t("gitlab:theMergeRequestIsStillLinked"),
        variant: "error",
      });
    }
  };

  return (
    <>
      <MRTopbarControl
        taskId={activeTaskId}
        mrs={mrs}
        gitlabAvailable={gitlabAvailable}
        compact={compact}
        mobile={mobile}
        onLink={() => setLinkOpen(true)}
        onUnlink={(associationId) => void unlink(associationId)}
      />
      <TaskMRLinkDialog
        open={linkOpen}
        onOpenChange={setLinkOpen}
        taskId={activeTaskId}
        workspaceId={workspaceId}
        taskRepositories={task?.repositories ?? EMPTY_TASK_REPOSITORIES}
        repositories={repositories}
      />
    </>
  );
});
